package oauth2

import (
	"crypto/subtle"

	"github.com/nelthaarion/breeze"
)

// Callback returns the handler for the provider's redirect_uri. It performs the
// entire back half of the OAuth2 dance automatically:
//
//  1. surface any provider error (?error=access_denied),
//  2. validate the state against the signed flow cookie (CSRF protection),
//  3. exchange the authorization code (with the PKCE verifier) for tokens,
//  4. fetch and normalize the user profile,
//  5. write the session (cookie or JWT, rotated),
//  6. redirect to SuccessRedirect (or the per-login ?redirect override).
//
// A downstream handler on the same route can read oauth2.CurrentUser(ctx) /
// oauth2.CurrentToken(ctx), because the callback also populates the context
// before finishing.
func Callback(cfg Config) breeze.HandlerFunc {
	c := prepareConfig(cfg)

	return func(ctx *breeze.Context) {
		// (1) Provider-side error (user denied consent, etc.).
		if e := ctx.Query("error"); e != "" {
			fail(ctx, c, 401, ErrProviderError)
			return
		}

		code := ctx.Query("code")
		returnedState := ctx.Query("state")
		if code == "" {
			fail(ctx, c, 400, ErrMissingCode)
			return
		}
		if returnedState == "" {
			fail(ctx, c, 400, ErrMissingState)
			return
		}

		// (2) Load + validate flow state (signed, unexpired) and compare the
		// state value in constant time. The flow cookie is single-use: we clear
		// it immediately so a replayed callback cannot succeed.
		fs, err := loadFlowState(ctx, c)
		if err != nil {
			fail(ctx, c, 401, err)
			return
		}
		clearFlowState(ctx, c)
		if subtle.ConstantTimeCompare([]byte(fs.State), []byte(returnedState)) != 1 {
			fail(ctx, c, 401, ErrStateMismatch)
			return
		}

		reqCtx, cancel := reqContext()
		defer cancel()

		// (3) Exchange the code (+ PKCE verifier) for tokens.
		tok, err := c.driver.Exchange(reqCtx, c, code, fs.Verifier)
		if err != nil {
			fail(ctx, c, 502, err)
			return
		}

		// (3b) If the provider returned an id_token, verify its nonce claim
		// matches the one we issued for this flow (ID token replay protection).
		if tok.IDToken != "" {
			nonce, err := idTokenNonce(tok.IDToken)
			if err != nil || subtle.ConstantTimeCompare([]byte(nonce), []byte(fs.Nonce)) != 1 {
				fail(ctx, c, 401, ErrNonceMismatch)
				return
			}
		}

		// (4) Fetch + normalize the user profile.
		user, err := c.driver.UserInfo(reqCtx, c, tok)
		if err != nil {
			fail(ctx, c, 502, err)
			return
		}

		// (5) Persist the session (rotated) and expose it on the context.
		if err := writeSession(ctx, c, user, tok); err != nil {
			fail(ctx, c, 500, err)
			return
		}
		setContext(ctx, &session{User: user, Token: tok})

		// (6) Redirect to the post-login destination.
		dest := c.SuccessRedirect
		if fs.Redirect != "" {
			dest = fs.Redirect
		}
		redirect(ctx, dest)
	}
}
