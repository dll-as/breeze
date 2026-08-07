package events

import "time"

// This file is the ENTIRE coupling surface between the event engine and
// any observability tooling. It deliberately defines its own small value
// types rather than importing a model from elsewhere, because the events
// package must stay dependency-free and usable on its own.
//
// The names here are prefixed to avoid colliding with [EventInfo] and
// [ListenerInfo], which the inspector already uses for a different
// purpose (static registry introspection rather than live dispatch).

// Observer receives notifications as dispatches move through the bus.
//
// It is the hook the observability layer attaches to. The events package
// never calls into a dashboard, a collector or a transport: it only ever
// invokes the four methods below on whatever value was handed to
// [Bus.SetObserver].
//
// # Cost when unused
//
// A bus with no observer performs one atomic pointer load per dispatch
// and nothing else — no allocation, no goroutine, no per-listener work.
// The pointer is loaded once at the start of a dispatch and threaded
// through, so the per-listener hooks cost nothing when absent.
//
// # Implementation contract
//
// Implementations MUST NOT block: every method runs inline on the
// dispatching goroutine, so slow work belongs on a queue owned by the
// observer. They MUST NOT panic; a panicking observer is recovered by
// the bus, but the panic is charged to the dispatch.
//
// They MUST NOT retain the *Context passed inside [DispatchInfo]: it is
// pooled and will be reused once the dispatch ends. Copy what you need.
//
// All four methods may be called concurrently from different goroutines.
type Observer interface {
	// OnEventStart fires once per dispatch, before any listener runs.
	OnEventStart(DispatchInfo)

	// OnEventEnd fires once per dispatch, after the last listener has
	// run (or after propagation stopped).
	OnEventEnd(DispatchResult)

	// OnListenerStart fires before each listener in the dispatch order is
	// considered. A listener that turns out to be skipped — by its filter
	// or by a spent once-guard — still reports a matching
	// [ListenerOutcome] with Skipped set, so starts and ends always pair
	// up.
	OnListenerStart(ListenerCall)

	// OnListenerEnd fires after each listener returns, including when it
	// panicked.
	OnListenerEnd(ListenerOutcome)
}

// DispatchInfo describes a dispatch that is about to begin.
type DispatchInfo struct {
	// EventID uniquely identifies this dispatch within the process.
	EventID uint64
	// EventName is the registered name of the event, or its Go type.
	EventName string
	// Time is the instant the dispatch started.
	Time time.Time
	// CorrelationID ties this dispatch to a wider logical operation.
	CorrelationID string
	// RequestID ties this dispatch to an inbound request.
	RequestID string
	// ListenerCount is the number of listeners in the snapshot. Some may
	// still be skipped by filters.
	ListenerCount int
	// PayloadSize is the in-memory width of the event value in bytes. It
	// is a compile-time constant per event type, so obtaining it costs
	// nothing and involves no reflection.
	PayloadSize int
	// Async reports whether this is an [EmitAsyncBus] dispatch.
	Async bool
}

// DispatchResult describes a dispatch that has finished.
type DispatchResult struct {
	// EventID matches the [DispatchInfo.EventID] of the same dispatch.
	EventID uint64
	// EventName is the registered name of the event, or its Go type.
	EventName string
	// Time is the instant the dispatch started.
	Time time.Time
	// Duration is how long the whole dispatch took, middleware included.
	Duration time.Duration
	// ListenersExecuted counts listeners that actually ran, excluding
	// those skipped by a filter or a spent once-guard.
	ListenersExecuted int
	// Cancelled reports whether propagation was stopped early, either by
	// a listener returning [Stop] or by [Context.Cancel].
	Cancelled bool
	// Err is the error the dispatch produced, or nil. [Stop] is never
	// reported here: stopping is signalled by Cancelled.
	Err error
	// CorrelationID ties this dispatch to a wider logical operation.
	CorrelationID string
	// RequestID ties this dispatch to an inbound request.
	RequestID string
	// Async reports whether this was an [EmitAsyncBus] dispatch.
	Async bool
	// Payload is the event value. It is populated only when the observer
	// asked for it via [Bus.SetObserverWithPayload]; otherwise it is nil,
	// so a dispatch never boxes its event into an interface by default.
	Payload any
}

// ListenerCall describes a listener that is about to run.
type ListenerCall struct {
	// EventID identifies the owning dispatch.
	EventID uint64
	// EventName is the registered name of the event, or its Go type.
	EventName string
	// ListenerName is the derived name of the handler.
	ListenerName string
	// ListenerID is the listener's registration id.
	ListenerID uint64
	// Priority is the listener's configured priority.
	Priority int
	// Phase is "before", "normal" or "after".
	Phase string
	// Index is the listener's position in the dispatch order.
	Index int
	// StartTime is the instant the listener was entered.
	StartTime time.Time
}

// ListenerOutcome describes a listener that has returned.
type ListenerOutcome struct {
	// EventID identifies the owning dispatch.
	EventID uint64
	// EventName is the registered name of the event, or its Go type.
	EventName string
	// ListenerName is the derived name of the handler.
	ListenerName string
	// ListenerID is the listener's registration id.
	ListenerID uint64
	// Priority is the listener's configured priority.
	Priority int
	// Phase is "before", "normal" or "after".
	Phase string
	// Index is the listener's position in the dispatch order, or -1 when
	// it ran asynchronously and has no deterministic position.
	//
	// The identity fields are repeated here rather than left only on
	// [ListenerCall] so that an observer which cares solely about
	// outcomes never has to correlate the two callbacks.
	Index int
	// Duration is how long the listener took.
	Duration time.Duration
	// Err is the error the listener returned, or nil. A listener that
	// stopped propagation reports [Stop] here.
	Err error
	// Panicked reports whether the listener panicked.
	Panicked bool
	// Skipped reports that the listener did not run, because its filter
	// rejected the event or its once-guard was already spent.
	Skipped bool
}

// SetObserver attaches o to the bus, replacing any previous observer.
// Passing nil detaches the current observer and restores the untouched
// fast path.
//
// Only one observer may be attached at a time. Fanning out to several
// consumers is the observability layer's job, not the bus's — keeping it
// out of here is what makes the hot path a single pointer load.
//
//	bus.SetObserver(myObserver)
//	defer bus.SetObserver(nil)
func (b *Bus) SetObserver(o Observer) {
	b.setObserver(o, false)
}

// SetObserverWithPayload attaches o and additionally delivers the event
// value in [DispatchResult.Payload].
//
// Carrying the payload boxes the event into an interface once per
// dispatch, which costs an allocation for event types that do not fit in
// a word. Use [SetObserver] unless the observer genuinely inspects
// payloads.
func (b *Bus) SetObserverWithPayload(o Observer) {
	b.setObserver(o, true)
}

// setObserver stores or clears the observer and its payload preference.
func (b *Bus) setObserver(o Observer, payload bool) {
	if o == nil {
		b.obs.Store(nil)
		b.obsPayload.Store(false)
		return
	}
	b.obsPayload.Store(payload)
	b.obs.Store(&o)
}

// Observer returns the currently attached observer, or nil.
func (b *Bus) Observer() Observer {
	return b.observer()
}

// observer loads the attached observer. This is the hot-path accessor:
// one atomic pointer load, and nil when nothing is attached.
func (b *Bus) observer() Observer {
	if p := b.obs.Load(); p != nil {
		return *p
	}
	return nil
}

// ObserverEnabled reports whether an observer is attached.
func (b *Bus) ObserverEnabled() bool { return b.obs.Load() != nil }

// notifyStart invokes OnEventStart with panic containment.
//
// An observer is third-party code as far as the bus is concerned, so a
// panic inside it must not escape into the emitting goroutine and must
// not abort the dispatch. It is reported through the configured panic
// handler like any other recovered panic.
func (b *Bus) notifyStart(o Observer, d DispatchInfo) {
	defer b.recoverObserver(d.EventName)
	o.OnEventStart(d)
}

// notifyEnd invokes OnEventEnd with panic containment.
func (b *Bus) notifyEnd(o Observer, d DispatchResult) {
	defer b.recoverObserver(d.EventName)
	o.OnEventEnd(d)
}

// notifyListenerStart invokes OnListenerStart with panic containment.
func (b *Bus) notifyListenerStart(o Observer, c ListenerCall) {
	defer b.recoverObserver(c.EventName)
	o.OnListenerStart(c)
}

// notifyListenerEnd invokes OnListenerEnd with panic containment.
func (b *Bus) notifyListenerEnd(o Observer, c ListenerOutcome) {
	defer b.recoverObserver(c.EventName)
	o.OnListenerEnd(c)
}

// recoverObserver swallows a panic raised by observer code and routes it
// to the bus's panic handler.
func (b *Bus) recoverObserver(event string) {
	r := recover()
	if r == nil {
		return
	}
	if b.cfg.OnPanic != nil {
		b.cfg.OnPanic(&PanicError{
			Event:    event,
			Listener: "<observer>",
			Value:    r,
		})
	}
}
