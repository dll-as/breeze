package oauth2

import "github.com/nelthaarion/breeze"

// Logout returns a handler that clears the session cookie and redirects to
// SuccessRedirect (or "/"). It also clears any lingering flow-state cookie so
// no OAuth artifacts survive logout.
//
//	app.Router.Handle(breeze.GET, "/logout/google", oauth2.Logout(cfg))
func Logout(cfg Config) breeze.HandlerFunc {
	c := prepareConfig(cfg)

	return func(ctx *breeze.Context) {
		clearSession(ctx, c)
		clearFlowState(ctx, c)

		dest := c.SuccessRedirect
		if r := ctx.Query("redirect"); isLocalRedirect(r) {
			dest = r
		}
		redirect(ctx, dest)
	}
}
