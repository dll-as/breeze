package events

import "context"

// This file mirrors the *Bus API onto the [Default] bus. These are the
// functions application code and plugins normally use:
//
//	events.On(UserCreated{}, handler)
//	events.Emit(UserCreated{UserID: 10})
//
// Each is a one-line delegation, so there is no behavioural difference
// between the package-level form and the explicit-bus form documented on
// the underlying function.

// On registers a listener for T on the default bus.
//
// The first argument is used only for type inference and is never read;
// pass a zero value:
//
//	events.On(UserCreated{}, func(ctx *events.Context, e UserCreated) error {
//		return sendWelcomeEmail(e.UserID)
//	})
//
// Use [OnType] when you would rather name T explicitly. The returned
// [Subscription] configures priority and filtering, and removes the
// listener.
func On[T any](sample T, fn Handler[T]) *Subscription[T] {
	return OnBus(Default, sample, fn)
}

// OnType registers a listener for T on the default bus with T given
// explicitly:
//
//	events.OnType[UserCreated](handler)
func OnType[T any](fn Handler[T]) *Subscription[T] {
	return OnTypeBus[T](Default, fn)
}

// Once registers a listener on the default bus that runs at most once and
// is then removed.
func Once[T any](sample T, fn Handler[T]) *Subscription[T] {
	return OnceBus(Default, sample, fn)
}

// OnceType registers a once-listener on the default bus with T given
// explicitly.
func OnceType[T any](fn Handler[T]) *Subscription[T] {
	return OnceTypeBus[T](Default, fn)
}

// Before registers a listener on the default bus in the before phase.
// Before-hooks run ahead of every normal listener for the event.
func Before[T any](sample T, fn Handler[T]) *Subscription[T] {
	return BeforeBus(Default, sample, fn)
}

// BeforeType registers a before-hook on the default bus with T given
// explicitly.
func BeforeType[T any](fn Handler[T]) *Subscription[T] {
	return BeforeTypeBus[T](Default, fn)
}

// After registers a listener on the default bus in the after phase.
// After-hooks run once every normal listener has finished.
func After[T any](sample T, fn Handler[T]) *Subscription[T] {
	return AfterBus(Default, sample, fn)
}

// AfterType registers an after-hook on the default bus with T given
// explicitly.
func AfterType[T any](fn Handler[T]) *Subscription[T] {
	return AfterTypeBus[T](Default, fn)
}

// Off removes the listener with the given id from T's registrations on
// the default bus. It reports whether a listener was removed.
func Off[T any](id uint64) bool {
	return OffBus[T](Default, id)
}

// Emit dispatches event synchronously on the default bus, returning once
// every eligible listener has run. See [EmitBus] for the error contract.
func Emit[T any](event T) error {
	return EmitBus(Default, event)
}

// EmitCtx is [Emit] with a caller-supplied context.Context, exposed to
// listeners as [Context.Ctx].
func EmitCtx[T any](ctx context.Context, event T) error {
	return EmitCtxBus(Default, ctx, event)
}

// EmitAsync dispatches event on the default bus without waiting for its
// listeners. Listener errors are reported through [Config.OnError].
func EmitAsync[T any](event T) error {
	return EmitAsyncBus(Default, event)
}

// EmitAsyncCtx is [EmitAsync] with a caller-supplied context.
func EmitAsyncCtx[T any](ctx context.Context, event T) error {
	return EmitAsyncCtxBus(Default, ctx, event)
}

// EmitAsyncWait dispatches asynchronously on the default bus and waits
// for every listener to finish.
func EmitAsyncWait[T any](event T) error {
	return EmitAsyncWaitBus(Default, event)
}

// Use appends middleware to the default bus's dispatch chain. Middleware
// registered first wraps middleware registered later.
func Use(mw ...Middleware) {
	Default.Use(mw...)
}

// SetName assigns a stable display name to T on the default bus:
//
//	events.SetName[UserCreated]("user.created")
//
// Names appear in the inspector, the recorder and the dashboard.
func SetName[T any](name string) {
	Name[T](Default, name)
}

// GetName returns the display name registered for T on the default bus,
// or its Go type name.
func GetName[T any]() string {
	return NameOf[T](Default)
}

// CountOf returns the number of listeners registered for T on the default
// bus.
func CountOf[T any]() int {
	return Count[T](Default)
}

// HasListeners reports whether T has at least one listener on the default
// bus.
func HasListeners[T any]() bool {
	return Has[T](Default)
}

// ClearListeners removes every listener registered for T on the default
// bus.
func ClearListeners[T any]() {
	Clear[T](Default)
}

// List returns the display names of every event type registered on the
// default bus, sorted alphabetically.
func List() []string {
	return Default.EventNames()
}

// CountEvents returns the number of event types the default bus has seen.
func CountEvents() int {
	return Default.EventCount()
}

// CountListeners returns the total number of listeners across all events
// on the default bus.
func CountListeners() int {
	return Default.ListenerCount()
}

// HasEvent reports whether an event with the given display name has at
// least one listener on the default bus.
func HasEvent(name string) bool {
	return Default.HasEvent(name)
}

// InspectEvent returns detailed information about T's listeners and
// metrics on the default bus.
func InspectEvent[T any]() EventInfo {
	return Inspect[T](Default)
}

// InspectAll returns information for every event type registered on the
// default bus, sorted by name.
//
// The per-type form is [InspectEvent]; the name Inspect itself is taken
// by the generic bus-scoped function.
func InspectAll() []EventInfo {
	return Default.InspectAll()
}

// MetricsOf returns the dispatch statistics for T on the default bus.
func MetricsOf[T any]() Metrics {
	return MetricsFor[T](Default)
}

// TotalMetrics returns the aggregated statistics across all event types
// on the default bus.
func TotalMetrics() Metrics {
	return Default.TotalMetrics()
}

// EnableRecorder turns the default bus's event recorder on.
func EnableRecorder() {
	Default.EnableRecorder()
}

// EnableRecorderWithPayload turns the recorder on and retains event
// values.
func EnableRecorderWithPayload() {
	Default.EnableRecorderWithPayload()
}

// DisableRecorder turns the default bus's recorder off.
func DisableRecorder() {
	Default.DisableRecorder()
}

// History returns the default bus's recorded dispatches, oldest first.
func History() []Record {
	return Default.RecorderHistory()
}

// ClearHistory discards the default bus's recorded history.
func ClearHistory() {
	Default.ClearRecorderHistory()
}

// Reset removes every listener and all middleware from the default bus
// and clears its recorder. It is intended for tests.
func Reset() {
	Default.Reset()
}
