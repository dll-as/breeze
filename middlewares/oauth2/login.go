package oauth2

import "github.com/nelthaarion/breeze"

// Login returns a handler that begins the OAuth2 authorization-code flow:
//
//  1. generate a cryptographically random state (+ nonce, + PKCE verifier),
//  2. persist them in a short-lived, signed, HttpOnly cookie,
//  3. redirect the browser to the provider's authorization URL.
//
// The developer wires it with a single line:
//
//	app.Router.Handle(breeze.GET, "/login/google", oauth2.Login(cfg))
//
// No manual redirect construction, state generation or PKCE handling is needed.
func Login(cfg Config) breeze.HandlerFunc {
	c := prepareConfig(cfg)

	return func(ctx *breeze.Context) {
		var verifier, challenge string
		if c.pkceEnabled() {
			p := newPKCE()
			verifier, challenge = p.Verifier, p.Challenge
		}

		fs := newFlowState(c, verifier)

		// Allow the caller to pass ?redirect=/somewhere to override where the
		// user lands after login (validated as a local path to prevent open
		// redirects).
		if r := ctx.Query("redirect"); isLocalRedirect(r) {
			fs.Redirect = r
		}

		fs.save(ctx, c)

		authURL := c.driver.AuthURL(c, fs.State, fs.Nonce, challenge)
		redirect(ctx, authURL)
	}
}

// isLocalRedirect reports whether p is a safe same-site path ("/foo"). It
// rejects absolute URLs and protocol-relative URLs ("//evil.com") to prevent
// open-redirect abuse.
func isLocalRedirect(p string) bool {
	if len(p) < 1 || p[0] != '/' {
		return false
	}
	if len(p) >= 2 && p[1] == '/' {
		return false // protocol-relative //host
	}
	return true
}
