package mc8pro

import (
	"log/slog"
	"sync"

	"github.com/benemon/morningstar-sdk/pkg/mc8pro/sysex"
)

// router dispatches incoming SysEx frames to interested subscribers.
//
// There is exactly ONE abstraction: a filtered subscription. Each
// subscriber registers a filter function and receives every frame
// for which the filter returns true. A nil filter means "match
// everything" and is used by the dump collector during Open.
//
// Request/reply patterns (Ping, fetchFirmware, future Client.Read*)
// express themselves as short-lived subscriptions: register a filter
// that matches the expected reply command, read one frame, cancel.
// They are not a separate primitive; they're the same subscription
// mechanism used with a one-shot consumption pattern at the caller.
//
// Delivery semantics: each dispatched frame is offered to every
// subscriber whose filter matches. Delivery is non-blocking — if a
// subscriber's channel buffer is full, the frame is dropped for that
// subscriber (and logged at debug level) so one slow consumer can't
// stall the entire receive goroutine.
type router struct {
	mu   sync.Mutex
	subs []*subscription
	log  *slog.Logger
}

// subscription is one active subscriber in the router. The once
// field guards the channel close so that cancel() and closeAll()
// can both be called against the same subscription without
// double-closing.
type subscription struct {
	filter func(sysex.Frame) bool
	ch     chan sysex.Frame
	once   sync.Once
}

// close closes the subscription's channel at most once. Safe to
// call from any goroutine.
func (s *subscription) close() {
	s.once.Do(func() { close(s.ch) })
}

// subscriberChanBuffer is the per-subscription channel buffer size.
// 256 is generous enough to hold the entire post-session-open proactive
// dump (~73 frames observed) without blocking the receive goroutine,
// and cheap enough that creating a short-lived one-shot subscription
// is effectively free.
const subscriberChanBuffer = 256

// newRouter creates a router with the given logger. The logger is
// used to emit debug messages when frames are dropped due to full
// subscriber buffers. If log is nil, a discard logger is used.
func newRouter(log *slog.Logger) *router {
	if log == nil {
		log = discardLogger()
	}
	return &router{log: log}
}

// subscribe registers a new subscriber and returns a channel that
// will receive every frame for which filter returns true. A nil
// filter matches every frame.
//
// The caller MUST eventually call cancel() to release the
// subscription. Typical usage:
//
//	ch, cancel := r.subscribe(func(f sysex.Frame) bool {
//	    return f.Cmd1 == 0x00 && f.Cmd2 == 0x7D
//	})
//	defer cancel()
//	// ... read from ch ...
//
// cancel is idempotent and safe to call from any goroutine.
func (r *router) subscribe(filter func(sysex.Frame) bool) (<-chan sysex.Frame, func()) {
	sub := &subscription{
		filter: filter,
		ch:     make(chan sysex.Frame, subscriberChanBuffer),
	}

	r.mu.Lock()
	r.subs = append(r.subs, sub)
	r.mu.Unlock()

	cancel := func() {
		r.mu.Lock()
		for i, s := range r.subs {
			if s == sub {
				r.subs = append(r.subs[:i], r.subs[i+1:]...)
				break
			}
		}
		r.mu.Unlock()
		sub.close()
	}

	return sub.ch, cancel
}

// expect is a convenience wrapper over subscribe for the common
// request/reply pattern: "wake me when a frame arrives with exactly
// this cmd1/cmd2 pair." The returned channel receives every matching
// frame; typical callers read one and cancel.
//
// It exists purely for ergonomic symmetry with Phase 2 code. It is
// identical in behavior to calling subscribe with an equality filter.
func (r *router) expect(cmd1, cmd2 byte) (<-chan sysex.Frame, func()) {
	return r.subscribe(func(f sysex.Frame) bool {
		return f.Cmd1 == cmd1 && f.Cmd2 == cmd2
	})
}

// dispatch offers a frame to every subscriber whose filter matches.
// Non-blocking: if a subscriber's channel is full, the frame is
// dropped for that subscriber only, and a debug log entry is emitted.
// Returns true if at least one subscriber received the frame.
func (r *router) dispatch(frame sysex.Frame) bool {
	r.mu.Lock()
	// Copy the active subscriber list so we can release the mutex
	// before sending on channels. Holding the mutex across a channel
	// send risks a deadlock if the channel is full (non-blocking
	// mitigates this but copying is clearer regardless).
	subs := make([]*subscription, len(r.subs))
	copy(subs, r.subs)
	r.mu.Unlock()

	var delivered bool
	for _, sub := range subs {
		if sub.filter != nil && !sub.filter(frame) {
			continue
		}
		select {
		case sub.ch <- frame:
			delivered = true
		default:
			r.log.Debug("subscriber buffer full; dropping frame",
				slog.String("component", "router"))
		}
	}
	return delivered
}

// closeAll cancels every active subscription by closing their
// channels. Blocked receivers unblock and see a zero-value frame
// followed by channel-closed. Safe to call concurrently with
// subscription cancels; each subscription's close is guarded by a
// sync.Once so double-closing is impossible.
//
// Used during Client.Close so no goroutine is left waiting on the
// router after shutdown.
func (r *router) closeAll() {
	r.mu.Lock()
	subs := r.subs
	r.subs = nil
	r.mu.Unlock()

	for _, sub := range subs {
		sub.close()
	}
}
