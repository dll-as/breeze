package events

import (
	"sync"
	"sync/atomic"
	"time"
)

// Record is one entry in the event history.
type Record struct {
	// EventID is the dispatch identifier from [Context.EventID].
	EventID uint64
	// Name is the event's registered or type name.
	Name string
	// Time is when the dispatch started.
	Time time.Time
	// Duration is how long the dispatch took.
	Duration time.Duration
	// Listeners is the number of listeners that actually ran.
	Listeners int
	// Async reports whether the dispatch was scheduled by EmitAsync. For
	// async dispatches Duration measures scheduling, not execution.
	Async bool
	// Stopped reports whether propagation was halted early.
	Stopped bool
	// Err is the error the dispatch returned, if any.
	Err error
	// CorrelationID and RequestID are copied from the Context.
	CorrelationID string
	RequestID     string
	// Payload is the event value. It is retained only when the recorder
	// was enabled with payload capture; see [Bus.EnableRecorderWithPayload].
	Payload any
}

// recorder is a fixed-capacity ring buffer of recent dispatches.
//
// It is intended for debugging and for the dashboard's live feed, and is
// off by default: recording costs a mutex acquisition and a struct copy
// per dispatch, which is significant relative to an otherwise
// lock-free path.
//
// The buffer overwrites its oldest entry when full, so a busy process
// keeps a bounded, recent window rather than growing without limit.
type recorder struct {
	mu      sync.Mutex
	entries []Record
	head    int // index of the oldest entry
	count   int
	cap     int

	// enabled is atomic so the dispatch path can check it without
	// touching the mutex.
	enabled atomic.Bool

	// payloads records whether event values should be retained. Payload
	// capture is opt-in because it extends the lifetime of every recorded
	// event value by up to `cap` dispatches.
	payloads atomic.Bool

	// total counts every record offered, including those later evicted,
	// so callers can tell how much history they have lost.
	total atomic.Uint64
}

// newRecorder returns a disabled recorder with the given capacity.
func newRecorder(capacity int) *recorder {
	if capacity < 1 {
		capacity = 1
	}
	return &recorder{
		entries: make([]Record, capacity),
		cap:     capacity,
	}
}

// enable turns recording on, optionally retaining payloads.
func (r *recorder) enable(payloads bool) {
	r.payloads.Store(payloads)
	r.enabled.Store(true)
}

// disable turns recording off. Existing history is retained so it can
// still be inspected after the fact.
func (r *recorder) disable() { r.enabled.Store(false) }

// isEnabled reports whether recording is on.
func (r *recorder) isEnabled() bool { return r.enabled.Load() }

// wantsPayload reports whether payloads should be captured.
func (r *recorder) wantsPayload() bool { return r.payloads.Load() }

// push appends rec, evicting the oldest entry when full.
func (r *recorder) push(rec Record) {
	r.mu.Lock()
	if r.count < r.cap {
		r.entries[(r.head+r.count)%r.cap] = rec
		r.count++
	} else {
		r.entries[r.head] = rec
		r.head = (r.head + 1) % r.cap
	}
	r.mu.Unlock()
	r.total.Add(1)
}

// history returns a copy of the buffer, oldest first.
func (r *recorder) history() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Record, r.count)
	for i := 0; i < r.count; i++ {
		out[i] = r.entries[(r.head+i)%r.cap]
	}
	return out
}

// clear discards the history.
//
// The entries are zeroed rather than merely unlinked so that retained
// payloads and errors become collectable immediately.
func (r *recorder) clear() {
	r.mu.Lock()
	for i := range r.entries {
		r.entries[i] = Record{}
	}
	r.head = 0
	r.count = 0
	r.mu.Unlock()
}

// stats returns the recorder's occupancy.
func (r *recorder) stats() RecorderStats {
	r.mu.Lock()
	count := r.count
	r.mu.Unlock()
	return RecorderStats{
		Enabled:  r.enabled.Load(),
		Payloads: r.payloads.Load(),
		Size:     count,
		Capacity: r.cap,
		Total:    r.total.Load(),
	}
}

// RecorderStats describes the recorder's state.
type RecorderStats struct {
	// Enabled reports whether recording is currently on.
	Enabled bool
	// Payloads reports whether event values are being retained.
	Payloads bool
	// Size is the number of records currently held.
	Size int
	// Capacity is the ring buffer's capacity.
	Capacity int
	// Total is the number of records ever written, including evicted
	// ones. Total minus Size is the number of records lost to eviction.
	Total uint64
}
