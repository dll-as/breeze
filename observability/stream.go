package observability

import (
	"sync"
	"sync/atomic"
)

// subscriberBuffer is how many signals a subscriber may fall behind
// before it starts losing them. It is small on purpose: the dashboard
// coalesces on a broadcast tick anyway, so a deep queue would only serve
// to deliver stale data late.
const subscriberBuffer = 256

// signalStream fans signals out to live subscribers.
//
// The critical property is that publishing never blocks. A subscriber
// that stops reading — a browser tab that froze, a WebSocket that stalled
// — must not be able to apply backpressure to the goroutine that is
// dispatching an event. Signals for a full subscriber are dropped and
// counted instead.
type signalStream struct {
	mu   sync.RWMutex
	subs map[uint64]*subscriber
	seq  atomic.Uint64

	dropped atomic.Uint64
	closed  atomic.Bool
}

type subscriber struct {
	ch   chan Signal
	once sync.Once
}

func newSignalStream() *signalStream {
	return &signalStream{subs: make(map[uint64]*subscriber)}
}

// subscribe registers a new listener and returns its channel plus the
// function that removes it. Calling unsubscribe twice is safe.
func (s *signalStream) subscribe() (<-chan Signal, func()) {
	sub := &subscriber{ch: make(chan Signal, subscriberBuffer)}

	if s.closed.Load() {
		close(sub.ch)
		return sub.ch, func() {}
	}

	id := s.seq.Add(1)
	s.mu.Lock()
	s.subs[id] = sub
	s.mu.Unlock()

	return sub.ch, func() { s.remove(id) }
}

// remove detaches a subscriber and closes its channel.
func (s *signalStream) remove(id uint64) {
	s.mu.Lock()
	sub := s.subs[id]
	delete(s.subs, id)
	s.mu.Unlock()

	if sub != nil {
		sub.once.Do(func() { close(sub.ch) })
	}
}

// publish delivers sig to every subscriber, skipping any that is full.
func (s *signalStream) publish(sig Signal) {
	if s.closed.Load() {
		return
	}
	s.mu.RLock()
	for _, sub := range s.subs {
		select {
		case sub.ch <- sig:
		default:
			// Subscriber is behind. Dropping is the only option that
			// keeps the emitting goroutine free.
			s.dropped.Add(1)
		}
	}
	s.mu.RUnlock()
}

// close detaches every subscriber and closes their channels.
func (s *signalStream) close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	s.mu.Lock()
	subs := s.subs
	s.subs = make(map[uint64]*subscriber)
	s.mu.Unlock()

	for _, sub := range subs {
		sub.once.Do(func() { close(sub.ch) })
	}
}

// count returns the number of live subscribers.
func (s *signalStream) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subs)
}

// Subscribers returns the number of live stream subscribers.
func (c *Collector) Subscribers() int { return c.stream.count() }

// Dropped returns how many signal deliveries were dropped because a
// subscriber could not keep up. A non-zero value means the dashboard is
// showing an incomplete live stream — the stored history in the ring
// buffer is unaffected.
func (c *Collector) Dropped() uint64 { return c.stream.dropped.Load() }
