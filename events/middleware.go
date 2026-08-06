package events

// Next is the continuation passed to a [Middleware]. Calling it runs the
// remainder of the chain and finally the listeners. Not calling it aborts
// the dispatch and whatever the middleware returns becomes Emit's result.
type Next func() error

// Middleware wraps an entire dispatch. It runs once per Emit rather than
// once per listener, which makes it the right place for concerns that
// describe the event as a whole: logging, tracing, recovery, metrics.
//
//	bus.Use(func(ctx *events.Context, next events.Next) error {
//		start := time.Now()
//		err := next()
//		log.Printf("%s took %s", ctx.EventName, time.Since(start))
//		return err
//	})
//
// Middleware is type-agnostic: it receives the [Context], which carries
// the event name and metadata, but not the event value. That is what lets
// one chain apply to every event type on the bus.
//
// Middleware registered first is the outermost wrapper, so Use(Logger)
// followed by Use(Recovery) produces Logger(Recovery(listeners)) — the
// same nesting order HTTP middleware stacks use.
type Middleware func(ctx *Context, next Next) error

// mwRunner walks a middleware chain.
//
// The obvious implementation composes the chain into nested closures, one
// allocation per middleware per dispatch. This walks an index instead:
// the only allocations are the runner and the single bound method value
// for `next`, regardless of how many middleware are registered.
//
// The runner is not reusable across dispatches because `i` is consumed as
// it walks; the dispatcher creates one per dispatch and only when the
// chain is non-empty.
type mwRunner struct {
	mw    []Middleware
	ctx   *Context
	final func() error
	i     int
}

// next runs the middleware at the current index, or the terminal function
// once the chain is exhausted.
//
// Advancing the index before the call is what makes a middleware's own
// invocation of next() reach the following middleware rather than
// recursing into itself.
func (r *mwRunner) next() error {
	if r.i >= len(r.mw) {
		return r.final()
	}
	m := r.mw[r.i]
	r.i++
	return m(r.ctx, r.next)
}

// runChain executes final wrapped in mw. It calls final directly when no
// middleware is registered, so the common case allocates nothing.
func runChain(mw []Middleware, ctx *Context, final func() error) error {
	if len(mw) == 0 {
		return final()
	}
	r := &mwRunner{mw: mw, ctx: ctx, final: final}
	return r.next()
}
