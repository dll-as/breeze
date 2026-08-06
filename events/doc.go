// Package events is the internal communication layer of the Breeze
// Framework.
//
// It provides a typed, allocation-conscious publish/subscribe system that
// every Breeze subsystem (router, middleware, OAuth2, plugins, dashboard,
// scheduler, websocket) can use to observe and extend behaviour without
// importing each other.
//
// # Events are plain structs
//
// An event is any Go type. No interface, no embedding, no registration
// step is required:
//
//	type UserCreated struct {
//		UserID uint64
//	}
//
// # Listening
//
//	sub := events.On(UserCreated{}, func(ctx *events.Context, e UserCreated) error {
//		return sendWelcomeEmail(e.UserID)
//	})
//	defer sub.Unsubscribe()
//
// The first argument exists only so the compiler can infer T; it is never
// read. Use [OnType] if you prefer explicit instantiation:
//
//	events.OnType[UserCreated](handler)
//
// # Emitting
//
//	err := events.Emit(UserCreated{UserID: 10})
//
// Listeners run in registration-sorted order: before-hooks first, then
// normal listeners by descending priority, then after-hooks. Sorting
// happens when a listener is registered or modified — never during a
// dispatch.
//
// # Type identity, not reflection
//
// The bus uses [reflect.Type] as a map key to identify an event type, and
// nothing else. Reflection is never used to read fields, build values, or
// call handlers: dispatch resolves to a strongly typed slice via a single
// type assertion per emit, and each handler is called through a direct
// func value.
//
// The key is derived with reflect.TypeOf((*T)(nil)).Elem(), which converts
// a nil pointer (pointer-shaped, so no boxing allocation) rather than a
// zero value of T.
//
// # Concurrency model
//
// Registration takes a mutex and publishes an immutable snapshot slice via
// atomic.Pointer. Dispatch loads that snapshot with a single atomic load
// and never takes a lock. Listeners added during a dispatch therefore do
// not affect the dispatch already in flight — it runs against the snapshot
// it started with.
//
// All exported APIs are safe for concurrent use.
//
// # Package-level vs explicit bus
//
// Every operation exists in two forms: a package-level function operating
// on the [Default] bus, and a *Bus form suffixed with "Bus" (or taking the
// bus as its first argument) for applications that want isolation:
//
//	events.Emit(e)              // default bus
//	events.EmitBus(myBus, e)    // explicit bus
//
// Go does not allow generic methods, which is why the typed operations are
// package-level functions taking a *Bus rather than methods on *Bus.
package events
