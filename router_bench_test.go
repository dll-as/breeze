package breeze

import "testing"

// BenchmarkFindChain measures the per-request routing cost on the hot path.
//
// The important number is "allocs/op". After precomputing the middleware
// chain at registration time, matching a route on the request path performs
// ZERO chain allocations for routes without :params (the previous code
// allocated a fresh []HandlerFunc on every single request). Routes with
// :params still take one pooled map, which sync.Pool amortises to ~0.
func benchRouter() *Router {
	r := NewRouter()
	r.autoServeRoot = false
	// A couple of global middlewares, like a real app (logging, CORS, ...).
	r.Use(func(*Context) {}, func(*Context) {})
	r.Handle(GET, "/users", func(*Context) {})
	r.Handle(GET, "/users/:id", func(*Context) {})
	r.Handle(POST, "/users", func(*Context) {})
	r.Handle(GET, "/health", func(*Context) {})
	return r
}

func BenchmarkFindChainStatic(b *testing.B) {
	r := benchRouter()
	req := &HTTPRequest{Method: GET, Path: "/health"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chain, _ := r.findChain(req)
		if chain == nil {
			b.Fatal("route not found")
		}
	}
}

func BenchmarkFindChainParam(b *testing.B) {
	r := benchRouter()
	req := &HTTPRequest{Method: GET, Path: "/users/42"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chain, params := r.findChain(req)
		if chain == nil {
			b.Fatal("route not found")
		}
		if params != nil {
			releaseParams(params) // return the pooled map, like releaseContext does
		}
	}
}

// TestFindChainComposition verifies the precomputed chain is
// [global..., route..., handler] in the correct order and length.
func TestFindChainComposition(t *testing.T) {
	r := NewRouter()
	r.autoServeRoot = false

	// Each middleware appends its tag then calls Next to advance the chain,
	// exactly like real middlewares do. The final handler does not call Next.
	var order []string
	r.Use(
		func(c *Context) { order = append(order, "g1"); c.Next() },
		func(c *Context) { order = append(order, "g2"); c.Next() },
	)
	r.Handle(GET, "/x", func(*Context) { order = append(order, "handler") },
		func(c *Context) { order = append(order, "r1"); c.Next() },
	)

	req := &HTTPRequest{Method: GET, Path: "/x"}
	chain, _ := r.findChain(req)
	if chain == nil {
		t.Fatal("expected a match for /x")
	}
	// 2 global + 1 route mw + 1 handler = 4
	if len(chain) != 4 {
		t.Fatalf("expected chain length 4, got %d", len(chain))
	}

	ctx := &Context{middlewares: chain, index: -1}
	ctx.Next()

	want := []string{"g1", "g2", "r1", "handler"}
	if len(order) != len(want) {
		t.Fatalf("expected %v, got %v", want, order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("at %d expected %q, got %q (full: %v)", i, want[i], order[i], order)
		}
	}
}

// TestUseRebuildsExistingChains verifies that calling Use AFTER routes are
// registered still applies the new global middleware to those routes (the
// chain is rebuilt), so ordering guarantees hold regardless of call order.
func TestUseRebuildsExistingChains(t *testing.T) {
	r := NewRouter()
	r.autoServeRoot = false

	var order []string
	r.Handle(GET, "/y", func(*Context) { order = append(order, "handler") })
	// Global middleware added AFTER the route. It calls Next to advance to
	// the handler, exactly like a real middleware.
	r.Use(func(c *Context) { order = append(order, "g-late"); c.Next() })

	req := &HTTPRequest{Method: GET, Path: "/y"}
	chain, _ := r.findChain(req)
	if chain == nil {
		t.Fatal("expected a match for /y")
	}
	ctx := &Context{middlewares: chain, index: -1}
	ctx.Next()

	want := []string{"g-late", "handler"}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		t.Fatalf("expected %v, got %v", want, order)
	}
}
