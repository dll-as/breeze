package events

import (
	"sync"
	"sync/atomic"
)

// channel holds every listener registered for one event type.
//
// # Copy-on-write
//
// Registrations are rare; dispatches are not. The channel therefore keeps
// a mutex-guarded master slice for writers and publishes an immutable,
// pre-sorted copy through an atomic pointer for readers. A dispatch does
// one atomic load and then walks a plain slice — no locks, no sorting, no
// bounds on how many dispatches run in parallel.
//
// A dispatch that is already running keeps using the snapshot it loaded,
// so listeners added or removed mid-dispatch take effect from the next
// dispatch onwards. That is the property that makes it safe for a
// listener to unsubscribe itself, or to register another listener, from
// inside a handler.
type channel[T any] struct {
	mu sync.Mutex

	// master is the writer-side source of truth, guarded by mu.
	master []*listener[T]

	// snapshot points at an immutable sorted copy of master. It is never
	// nil after construction.
	snapshot atomic.Pointer[[]*listener[T]]

	// size mirrors len(master) so readers can report the listener count
	// without taking the lock or dereferencing the snapshot.
	size atomic.Int64
}

// newChannel returns an empty channel with a published empty snapshot,
// so the dispatch path never has to nil-check the pointer.
func newChannel[T any]() *channel[T] {
	c := &channel[T]{}
	empty := make([]*listener[T], 0)
	c.snapshot.Store(&empty)
	return c
}

// load returns the current snapshot. Callers must treat it as read-only.
func (c *channel[T]) load() []*listener[T] {
	return *c.snapshot.Load()
}

// len returns the number of registered listeners.
func (c *channel[T]) len() int { return int(c.size.Load()) }

// publishLocked rebuilds and publishes the snapshot. The caller must hold
// c.mu.
//
// The copy is deliberate: publishing `master` directly would let a later
// append with spare capacity mutate a slice that running dispatches are
// still reading.
func (c *channel[T]) publishLocked() {
	next := make([]*listener[T], len(c.master))
	copy(next, c.master)
	sortListeners(next)
	c.snapshot.Store(&next)
	c.size.Store(int64(len(next)))
}

// add registers l and republishes the snapshot.
func (c *channel[T]) add(l *listener[T]) {
	c.mu.Lock()
	c.master = append(c.master, l)
	c.publishLocked()
	c.mu.Unlock()
}

// remove deletes the listener with the given id.
// It reports whether a listener was found.
func (c *channel[T]) remove(id uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, l := range c.master {
		if l.id != id {
			continue
		}
		// Order within master does not matter — publishLocked sorts the
		// snapshot — so delete by shifting the tail down. The slice is
		// small and this keeps the master free of nil holes.
		c.master = append(c.master[:i], c.master[i+1:]...)
		c.publishLocked()
		return true
	}
	return false
}

// update applies mutate to a clone of the listener with the given id and
// republishes. Cloning keeps the listener that running dispatches hold
// untouched; they finish against the values they started with.
func (c *channel[T]) update(id uint64, mutate func(*listener[T])) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, l := range c.master {
		if l.id != id {
			continue
		}
		cl := l.clone()
		mutate(cl)
		c.master[i] = cl
		c.publishLocked()
		return true
	}
	return false
}

// has reports whether a listener with the given id is registered.
func (c *channel[T]) has(id uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.master {
		if l.id == id {
			return true
		}
	}
	return false
}

// clearAll removes every listener.
func (c *channel[T]) clearAll() {
	c.mu.Lock()
	c.master = nil
	c.publishLocked()
	c.mu.Unlock()
}

// pruneFired drops once-listeners that have already run.
//
// A spent once-listener is skipped in O(1) during dispatch, so pruning is
// an occupancy optimisation rather than a correctness requirement. It runs
// after a dispatch observes at least one spent listener, which keeps a
// long-lived bus from accumulating dead entries.
func (c *channel[T]) pruneFired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.master[:0]
	changed := false
	for _, l := range c.master {
		if l.once && l.fired.Load() {
			changed = true
			continue
		}
		kept = append(kept, l)
	}
	if !changed {
		return
	}
	// Zero the vacated tail so the removed listeners (and anything their
	// closures captured) become collectable.
	for i := len(kept); i < len(c.master); i++ {
		c.master[i] = nil
	}
	c.master = kept
	c.publishLocked()
}

// describe returns an inspector view of every listener in execution
// order. It reads the snapshot, so it never blocks a dispatch.
func (c *channel[T]) describe() []ListenerInfo {
	snap := c.load()
	out := make([]ListenerInfo, 0, len(snap))
	for i, l := range snap {
		info := ListenerInfo{
			ID:       l.id,
			Name:     l.name,
			Priority: l.priority,
			Phase:    l.phase.String(),
			Order:    i,
			Once:     l.once,
			Filtered: l.filter != nil,
			Calls:    l.calls.Load(),
		}
		if l.once {
			info.Fired = l.fired.Load()
		}
		out = append(out, info)
	}
	return out
}
