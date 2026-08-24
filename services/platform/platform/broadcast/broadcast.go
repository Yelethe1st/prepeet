// Package broadcast fans a small signal out to every process that is listening
// for it.
//
// It exists because of a specific problem and a specific commitment. The
// problem: a candidate's interview progress is produced by whichever ECS task
// holds the workflow, and must reach the browser connected to whichever task
// holds the socket. Those are usually different tasks, so the signal has to
// leave the process.
//
// The commitment is in ADR-0006, which defers Redis behind four triggers and
// names this one as the most likely to fire first. That deferral is only
// defensible if the swap is cheap, and it is only cheap if nothing calls
// LISTEN/NOTIFY directly. So this interface exists before the streaming handler
// does, rather than being extracted from it afterwards.
//
// # What this is not
//
// It is not durable, and it is not a queue.
//
// Nothing is stored, so a message published while nobody was subscribed is
// gone rather than waiting, and nothing is ever replayed to a later subscriber.
// The contract suite asserts that loss rather than tolerating it. Anything that
// must not be lost goes through platform/outbox, which is durable by
// construction. Keeping the two apart is what stops a best effort transport from
// quietly becoming the system of record for something.
//
// At the boundary the guarantee is genuinely "may or may not". A message
// published in the moment before a subscription opens can go either way, and a
// caller must not depend on either outcome. That is weaker than it sounds only
// if you were already treating delivery as certain, which is the thing this
// package will not let you do.
//
// Delivery is to every subscriber, not to one of them. A work queue would need
// exactly-once claiming, which is what the outbox already does properly.
//
// # What may be carried
//
// Identifiers and small signals. Never restricted content: this is a fan-out to
// every process, subject to the same rule as telemetry in
// docs/security/data-classification.md. A subscriber that needs the content
// reads it from the database using the identifier, under the row-level security
// that decides whether it may.
//
// Implements part of PLT-08's fan-out prerequisite and the reversibility
// commitment in ADR-0006.
package broadcast

import (
	"context"
	"errors"
	"fmt"
)

// MaxPayload is the largest payload any implementation accepts.
//
// It is the intersection of what every implementation can carry, not what the
// current one can. PostgreSQL's NOTIFY has a hard 8000 byte ceiling and Redis
// has none, so a payload accepted by one and refused by the other would turn
// the swap ADR-0006 relies on into a breaking change discovered at runtime.
// Four thousand leaves room for the encoding overhead NOTIFY adds, and is far
// above anything an identifier needs.
const MaxPayload = 4000

// maxTopicLength is PostgreSQL's identifier limit. A channel name longer than
// this is silently truncated rather than rejected, so two topics differing only
// past the limit would become one.
const maxTopicLength = 63

var (
	// ErrInvalidTopic means the topic is not a name every implementation can use.
	ErrInvalidTopic = errors.New("broadcast: topic is not usable")
	// ErrPayloadTooLarge means the payload exceeds MaxPayload.
	ErrPayloadTooLarge = errors.New("broadcast: payload is too large")
	// ErrClosed means the subscription has already been closed.
	ErrClosed = errors.New("broadcast: subscription is closed")
)

// Message is one delivered signal.
type Message struct {
	// Topic is named on the message so a consumer holding several subscriptions
	// can tell them apart without threading the topic alongside the channel.
	Topic   string
	Payload []byte
}

// Subscription is an open stream of messages on one topic.
type Subscription interface {
	// Messages delivers signals until the subscription is closed or its context
	// is cancelled, at which point the channel is closed. Closing rather than
	// merely stopping matters: a consumer ranging over it must return.
	Messages() <-chan Message

	// Close ends the subscription. It is safe to call more than once, because
	// shutdown paths close things twice.
	Close() error
}

// Broadcaster publishes signals and subscribes to them.
//
// The interface is deliberately this small. Anything richer, such as
// acknowledgement, replay or ordering, would be a guarantee only some
// implementations can make, and the point of the interface is that the
// implementations are interchangeable.
type Broadcaster interface {
	// Publish sends a signal to every current subscriber of topic. It does not
	// block on subscribers and does not report how many received it: a producer
	// that depended on a consumer existing would be depending on delivery this
	// transport does not promise.
	Publish(ctx context.Context, topic string, payload []byte) error

	// Subscribe opens a stream. The subscription ends when it is closed or when
	// ctx is cancelled, whichever happens first.
	Subscribe(ctx context.Context, topic string) (Subscription, error)
}

// ValidateTopic reports whether a topic is usable by every implementation.
//
// The rules are PostgreSQL's, because they are the strictest: an unquoted
// identifier, lower case, at most 63 bytes. Redis would accept far more, and
// accepting more here would mean topics that work until the day the
// implementation changes.
//
// It validates rather than escapes for a harder reason. A topic name reaches
// LISTEN, which cannot take a bound parameter, so the name is concatenated into
// SQL. Escaping is a mechanism that can be got subtly wrong; refusing anything
// that is not already a bare identifier cannot be.
func ValidateTopic(topic string) error {
	if topic == "" {
		return fmt.Errorf("%w: a topic is required", ErrInvalidTopic)
	}
	if len(topic) > maxTopicLength {
		return fmt.Errorf("%w: %d bytes exceeds the %d byte identifier limit, past which names collide",
			ErrInvalidTopic, len(topic), maxTopicLength)
	}
	for i := 0; i < len(topic); i++ {
		c := topic[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		case c == '_' && i > 0:
		default:
			// The message names the position rather than echoing the whole
			// topic, since a rejected topic may be attacker supplied.
			return fmt.Errorf("%w: byte %d is not permitted; a topic is lower case letters, "+
				"digits and underscores, starting with a letter", ErrInvalidTopic, i)
		}
	}
	return nil
}

// validatePublish checks both arguments in the order a caller would hit them.
func validatePublish(topic string, payload []byte) error {
	if err := ValidateTopic(topic); err != nil {
		return err
	}
	if len(payload) > MaxPayload {
		return fmt.Errorf("%w: %d bytes exceeds the %d byte limit; carry an identifier and let the "+
			"subscriber read the rest", ErrPayloadTooLarge, len(payload), MaxPayload)
	}
	return nil
}
