// Package oauth2 provides zero-configuration OAuth2 authentication middleware
// for the Breeze framework. It supports Google, GitHub, Microsoft and Discord
// out of the box with automatic state, PKCE, token exchange, user-info
// normalization and session management (cookie or JWT).
//
// The developer only needs to supply a ClientID and ClientSecret; everything
// else has a secure default.
package oauth2

import "errors"

// Sentinel errors returned across the package. They are wrapped with context
// where useful, so callers should compare with errors.Is.
var (
	// ErrMissingCredentials is returned when ClientID or ClientSecret is empty.
	ErrMissingCredentials = errors.New("oauth2: ClientID and ClientSecret are required")

	// ErrUnknownProvider is returned when a Config references a Provider that
	// has no registered driver.
	ErrUnknownProvider = errors.New("oauth2: unknown provider")

	// ErrStateMismatch is returned when the state returned by the provider does
	// not match the state stored for the request (CSRF protection).
	ErrStateMismatch = errors.New("oauth2: state mismatch")

	// ErrMissingCode is returned when the callback does not include an
	// authorization code.
	ErrMissingCode = errors.New("oauth2: missing authorization code")

	// ErrMissingState is returned when the callback does not include a state
	// parameter or the corresponding state cookie is absent.
	ErrMissingState = errors.New("oauth2: missing state")

	// ErrProviderError is returned when the provider responds with an OAuth
	// error (e.g. access_denied) on the callback.
	ErrProviderError = errors.New("oauth2: provider returned an error")

	// ErrTokenExchange is returned when the authorization-code exchange fails.
	ErrTokenExchange = errors.New("oauth2: token exchange failed")

	// ErrUserInfo is returned when fetching the user profile fails.
	ErrUserInfo = errors.New("oauth2: failed to fetch user info")

	// ErrNoSession is returned by Auth when a request has no valid session.
	ErrNoSession = errors.New("oauth2: no valid session")

	// ErrNoRefreshToken is returned by Refresh when no refresh token is present.
	ErrNoRefreshToken = errors.New("oauth2: no refresh token available")

	// ErrExpiredState is returned when a stored state/PKCE cookie has expired.
	ErrExpiredState = errors.New("oauth2: state expired")
)
