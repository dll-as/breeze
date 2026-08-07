package observability

import "sync"

// ringBuffer is a fixed-capacity, thread-safe buffer of ordered values.
//
// It mirrors the shape the dashboard already uses for requests and logs:
// append-only, oldest entry evicted at capacity, and a snapshot that
// returns a copy in insertion order. A mutex rather than a lock-free
// scheme keeps it trivially correct, and the contention is low because
// reads happen on the dashboard's broadcast tick rather than per signal.
type ringBuffer[T any] struct {
	mu       sync.Mutex
	entries  []T
	head     int // index of the oldest entry
	count    int // number of valid entries
	capacity int
}

func newRingBuffer[T any](capacity int) *ringBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &ringBuffer[T]{
		entries:  make([]T, capacity),
		capacity: capacity,
	}
}

// Push appends v, evicting the oldest entry if the buffer is full.
func (r *ringBuffer[T]) Push(v T) {
	r.mu.Lock()
	if r.count < r.capacity {
		r.entries[(r.head+r.count)%r.capacity] = v
		r.count++
	} else {
		r.entries[r.head] = v
		r.head = (r.head + 1) % r.capacity
	}
	r.mu.Unlock()
}

// Snapshot returns a copy of all entries in insertion order (oldest
// first).
func (r *ringBuffer[T]) Snapshot() []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]T, r.count)
	for i := 0; i < r.count; i++ {
		out[i] = r.entries[(r.head+i)%r.capacity]
	}
	return out
}

// Len returns the current number of entries.
func (r *ringBuffer[T]) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// Cap returns the buffer's fixed capacity.
func (r *ringBuffer[T]) Cap() int { return r.capacity }

// Clear removes all entries.
func (r *ringBuffer[T]) Clear() {
	r.mu.Lock()
	r.head = 0
	r.count = 0
	r.mu.Unlock()
}
