package broadcast

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// channelName is the single PostgreSQL channel every signal travels on.
//
// One channel rather than one per topic, which is the decision this
// implementation turns on.
//
// LISTEN cannot be issued while a connection is blocked waiting for a
// notification, so a topic-per-channel design needs either a connection per
// subscription, which is one database connection per live interview, or a
// scheme for interrupting the waiting connection to register new channels. The
// first exhausts the pool at a few hundred interviews. The second is a
// concurrency problem with several ways to leave a connection in a state pgx
// considers broken.
//
// Sending everything down one channel and routing in process avoids both.
// Subscribing costs no database round trip at all, so a browser connecting is
// not a database operation. The price is that every task receives every
// message and discards the ones it has no subscriber for.
//
// That price is exactly the trigger ADR-0006 names. When fan-out volume makes
// filtering-everywhere wasteful, that is the measurement that says to move to
// Redis, where subscribing is genuinely per-topic. Until then this costs one
// connection for the whole process.
const channelName = "prepeet_broadcast"

// reconnectBackoff is how long to wait before rebuilding a dropped listener.
//
// Fixed rather than exponential, and short. The listener is one connection, so
// reconnecting cheaply is affordable, and the window while it is down is a
// window where signals are silently lost. Anything that cannot tolerate that
// loss is not supposed to be using this package.
const reconnectBackoff = 2 * time.Second

// envelope is what actually travels on the wire.
//
// The logical topic rides inside the payload because there is only one channel.
// The payload is base64 encoded because a NOTIFY payload is text, and a signal
// carrying a raw byte would otherwise fail at the database rather than at the
// call site, and only for some inputs. Encoding costs about a third more bytes,
// which MaxPayload already accounts for.
type envelope struct {
	Topic   string `json:"t"`
	Payload string `json:"p"`
}

// Postgres fans out across processes using LISTEN/NOTIFY.
//
// This is the deployed implementation. ECS Fargate runs several tasks, so a
// signal that never leaves the process reaches roughly one subscriber in
// however many tasks are running, and that failure is invisible in development
// where there is one task.
//
// Delivery is best effort, and this implementation makes that concrete: a
// message published while the listener is reconnecting is gone. Anything
// durable goes through platform/outbox.
type Postgres struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	// local does the in-process half. Once a notification arrives, delivering
	// it to this process's subscribers is exactly what Memory already does, and
	// duplicating that logic would mean two fan-out implementations to keep
	// honest.
	local *Memory

	cancel  context.CancelFunc
	stopped chan struct{}
	once    sync.Once
}

// NewPostgres builds a cross-process broadcaster and starts listening.
//
// It returns once the listener is established, so a Subscribe followed
// immediately by a Publish from another process is not a race. Establishing it
// lazily would make the first message after startup the one that goes missing.
func NewPostgres(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (*Postgres, error) {
	if log == nil {
		log = slog.Default()
	}

	bus := &Postgres{
		pool:    pool,
		log:     log,
		local:   NewMemory(),
		stopped: make(chan struct{}),
	}

	// The first connection is made synchronously so a configuration or
	// permission problem is a startup failure rather than a silence discovered
	// when somebody notices progress is not updating.
	conn, err := bus.listen(ctx)
	if err != nil {
		return nil, err
	}

	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	bus.cancel = cancel

	go bus.receive(lifetime, conn)

	return bus, nil
}

// listen acquires a connection and puts it into listening mode.
func (p *Postgres) listen(ctx context.Context) (*pgxpool.Conn, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("broadcast: acquiring the listener connection: %w", err)
	}

	// The channel name is a compile-time constant, so this concatenation
	// carries no input. Topic names, which do carry input, never reach SQL:
	// they travel inside the payload.
	if _, err := conn.Exec(ctx, "LISTEN "+channelName); err != nil {
		conn.Release()
		return nil, fmt.Errorf("broadcast: listening on %s: %w", channelName, err)
	}
	return conn, nil
}

// receive delivers notifications until the broadcaster is closed, rebuilding
// the connection whenever it drops.
//
// A dropped listener is not an error to report and give up on. It happens on
// database failover and on any connection reaper in between, and a broadcaster
// that stopped listening after one of those would leave every browser watching
// a live interview silently stuck.
func (p *Postgres) receive(ctx context.Context, conn *pgxpool.Conn) {
	defer close(p.stopped)
	defer func() {
		if conn != nil {
			conn.Release()
		}
	}()

	for {
		if conn == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectBackoff):
			}

			replacement, err := p.listen(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				p.log.Error("broadcast listener could not reconnect",
					slog.String("error", err.Error()))
				continue
			}
			conn = replacement
			p.log.Info("broadcast listener reconnected")
		}

		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// The underlying connection is closed before release, because one
			// that failed mid-wait must not go back into the pool for the next
			// query to fail on. Release alone would return it.
			p.log.Warn("broadcast listener lost its connection",
				slog.String("error", err.Error()))
			_ = conn.Conn().Close(context.WithoutCancel(ctx))
			conn.Release()
			conn = nil
			continue
		}

		p.deliver(ctx, notification.Payload)
	}
}

// deliver routes one received notification to this process's subscribers.
func (p *Postgres) deliver(ctx context.Context, raw string) {
	var received envelope
	if err := json.Unmarshal([]byte(raw), &received); err != nil {
		// A malformed notification is dropped rather than fatal: the channel is
		// shared, and another process on a different version is a likelier
		// cause than a bug here. The payload is not logged, since it is not
		// this package's to classify.
		p.log.Warn("broadcast received a notification it could not read")
		return
	}

	payload, err := base64.StdEncoding.DecodeString(received.Payload)
	if err != nil {
		p.log.Warn("broadcast received a notification with an unreadable payload",
			slog.String("topic", received.Topic))
		return
	}

	// Publish rather than a direct write, so cross-process delivery goes
	// through exactly the same in-process path as Memory and inherits its
	// non-blocking send.
	if err := p.local.Publish(ctx, received.Topic, payload); err != nil {
		p.log.Warn("broadcast could not route a notification",
			slog.String("topic", received.Topic), slog.String("error", err.Error()))
	}
}

// Publish sends a signal to every subscriber in every process.
func (p *Postgres) Publish(ctx context.Context, topic string, payload []byte) error {
	channel, body, err := NotifyArguments(topic, payload)
	if err != nil {
		return err
	}

	// pg_notify rather than NOTIFY, because it takes the channel and payload as
	// bound parameters. NOTIFY takes neither.
	if _, err := p.pool.Exec(ctx, "SELECT pg_notify($1, $2)", channel, body); err != nil {
		return fmt.Errorf("broadcast: publishing to %q: %w", topic, err)
	}
	return nil
}

// NotifyArguments returns the channel and payload for a pg_notify call.
//
// It exists for one caller: a publisher that must emit its signal inside its
// own transaction, so the signal becomes visible exactly when the row it
// describes does. PostgreSQL holds a notification until commit and drops it on
// rollback, which no external transport can do, and platform/outbox depends on
// that.
//
// Such a caller cannot use Publish, which runs on the pool rather than in the
// transaction. It must not reimplement the wire format either: a second copy of
// the encoding is a second thing to change, and the version that is not changed
// keeps publishing messages the listener silently discards. So the format lives
// here and is handed out.
func NotifyArguments(topic string, payload []byte) (channel, body string, err error) {
	if err := validatePublish(topic, payload); err != nil {
		return "", "", err
	}

	encoded, err := json.Marshal(envelope{
		Topic:   topic,
		Payload: base64.StdEncoding.EncodeToString(payload),
	})
	if err != nil {
		return "", "", fmt.Errorf("broadcast: encoding the notification: %w", err)
	}
	return channelName, string(encoded), nil
}

// Subscribe opens a stream on a topic.
//
// No database work happens here. Subscribing is a local registration, and the
// single listening connection is already receiving everything, so a browser
// connecting to a live interview costs nothing at the database.
func (p *Postgres) Subscribe(ctx context.Context, topic string) (Subscription, error) {
	return p.local.Subscribe(ctx, topic)
}

// Close stops listening. It is safe to call more than once.
func (p *Postgres) Close() error {
	p.once.Do(func() {
		p.cancel()
		select {
		case <-p.stopped:
		case <-time.After(5 * time.Second):
			// The listener is blocked somewhere it should not be. Waiting
			// longer during shutdown is worse than reporting it and exiting.
			p.log.Warn("broadcast listener did not stop within the shutdown window")
		}
	})
	return nil
}

// Compile-time proof that both implementations satisfy the same interface. The
// contract suite checks they behave the same; this checks they are substitutable
// at all, which is what the wiring depends on.
var (
	_ Broadcaster = (*Memory)(nil)
	_ Broadcaster = (*Postgres)(nil)
)
