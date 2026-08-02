package oauth2

import "github.com/nelthaarion/breeze"

// Optional returns a middleware that attaches the session when present but never
// blocks the request. Handlers can branch on oauth2.CurrentUser(ctx) != nil to
// render personalized-or-anonymous content from a single route.
//
//	app.Router.Handle(breeze.GET, "/", HomeHandler, oauth2.Optional(cfg))
func Optional(cfg Config) breeze.HandlerFunc {
	c := prepareConfig(cfg)

	return func(ctx *breeze.Context) {
		if s, err := readSession(ctx, c); err == nil {
			setContext(ctx, s)
		}
		// Always continue, authenticated or not.
		ctx.Next()
	}
}
