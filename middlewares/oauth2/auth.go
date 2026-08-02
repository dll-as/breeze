package oauth2

import "github.com/nelthaarion/breeze"

// Auth returns a middleware that requires a valid session. On success it
// populates the context (CurrentUser/CurrentToken) and calls the next handler.
// On failure it redirects to FailureRedirect when set, otherwise writes 401 —
// and never calls the protected handler.
//
//	app.Router.Handle(breeze.GET, "/dashboard",
//	    DashboardHandler,
//	    oauth2.Auth(cfg), // as a route middleware
//	)
func Auth(cfg Config) breeze.HandlerFunc {
	c := prepareConfig(cfg)

	return func(ctx *breeze.Context) {
		s, err := readSession(ctx, c)
		if err != nil {
			fail(ctx, c, 401, ErrNoSession)
			return
		}
		setContext(ctx, s)
		ctx.Next()
	}
}
