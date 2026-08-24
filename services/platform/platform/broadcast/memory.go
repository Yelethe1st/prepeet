package broadcast

import (
	"context"
	"sync"
)

// bufferPerSubscriber is how many messages a subscriber may fall behind by
// before it starts losing them.
//
// A buffer at all, because a subscriber briefly busy should not lose a signal.
// A small one, because a subscriber that is persistently behind is not going to
// catch up, and holding a long queue for it converts a slow consumer into
// unbounded memory. Dropping is the correct failure here: this transport does
// not promise delivery, and the alternative, blocking the publisher, would make
// one slow browser everyone's problem.
const bufferPerSubscriber = 64

// Memory fans out within one process.
//
// It is the implementation for tests and for single-instance local development,
// and it is deliberately not the deployed one: ECS Fargate runs several tasks,
// so a signal that never leaves the process reaches roughly one subscriber in
// however many tasks are running. That failure is invisible in development,
// where there is one task, which is exactly why the deployed wiring uses
// Postgres and this type stays in tests.
type Memory struct {
	mu     sync.Mutex
	topics map[string]map[*memorySubscription]struct{}
}

// NewMemory builds an in-process broadcaster.
func NewMemory() *Memory {
	return &Memory{topics: make(map[string]map[*memorySubscription]struct{})}
}

// Publish delivers to every current subscriber, dropping for any that is full.
func (m *Memory) Publish(_ context.Context, topic string, payload []byte) error {
	if err := validatePublish(topic, payload); err != nil {
		return err
	}

	m.mu.Lock()
	subscribers := make([]*memorySubscription, 0, len(m.topics[topic]))
	for subscriber := range m.topics[topic] {
		subscribers = append(subscribers, subscriber)
	}
	m.mu.Unlock()

	// The payload is copied per subscriber. Handing the same slice to several
	// consumers would let one of them mutate what the others read, and the
	// PostgreSQL implementation cannot do that, so neither may this one.
	for _, subscriber := range subscribers {
		copied := make([]byte, len(payload))
		copy(copied, payload)
		subscriber.deliver(Message{Topic: topic, Payload: copied})
	}
	return nil
}

// Subscribe opens an in-process subscription.
func (m *Memory) Subscribe(ctx context.Context, topic string) (Subscription, error) {
	if err := ValidateTopic(topic); err != nil {
		return nil, err
	}

	subscription := &memorySubscription{
		bus:      m,
		topic:    topic,
		messages: make(chan Message, bufferPerSubscriber),
		done:     make(chan struct{}),
	}

	m.mu.Lock()
	if m.topics[topic] == nil {
		m.topics[topic] = make(map[*memorySubscription]struct{})
	}
	m.topics[topic][subscription] = struct{}{}
	m.mu.Unlock()

	// Context cancellation ends the subscription without the caller closing it,
	// so a request scoped subscriber cannot outlive its request.
	go func() {
		select {
		case <-ctx.Done():
			_ = subscription.Close()
		case <-subscription.done:
		}
	}()

	return subscription, nil
}

// Subscribers reports how many subscriptions are open on a topic. It exists for
// tests that need to assert the registry does not leak.
func (m *Memory) Subscribers(topic string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.topics[topic])
}

type memorySubscription struct {
	bus      *Memory
	topic    string
	messages chan Message
	done     chan struct{}

	// mu guards closed and the send on messages together.
	//
	// Deregistering under the bus lock is not enough on its own. Publish
	// snapshots its subscribers, releases the bus lock and then sends, so a
	// Close landing in that window would close the channel while a send was in
	// flight, which panics. This lock is what makes delivery and closure
	// mutually exclusive, and the concurrency test in memory_test.go is what
	// found that the first version had it wrong.
	mu     sync.Mutex
	closed bool
}

func (s *memorySubscription) Messages() <-chan Message { return s.messages }

// deliver sends without blocking, and does nothing once closed.
func (s *memorySubscription) deliver(message Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	// See bufferPerSubscriber for why a full subscriber loses the message
	// rather than the publisher waiting for it.
	select {
	case s.messages <- message:
	default:
	}
}

func (s *memorySubscription) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil // shutdown paths close things twice
	}
	s.closed = true
	close(s.done)
	close(s.messages)
	s.mu.Unlock()

	// Deregistration happens after the subscription is marked closed, so a
	// Publish already holding this subscriber in its snapshot finds it closed
	// and drops rather than sending.
	s.bus.mu.Lock()
	delete(s.bus.topics[s.topic], s)
	// The topic is removed once empty, so a process that subscribes per session
	// does not accumulate one empty map per session it has ever served.
	if len(s.bus.topics[s.topic]) == 0 {
		delete(s.bus.topics, s.topic)
	}
	s.bus.mu.Unlock()

	return nil
}
