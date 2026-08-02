package oauth2

import (
	"net/http"
	"time"
)

// SessionMode selects how the authenticated session is persisted between the
// callback and subsequent requests.
type SessionMode int

const (
	// SessionModeCookie stores an opaque, signed session cookie server-side
	// semantics: the normalized User + Token are serialized into a signed,
	// HttpOnly cookie. This is the default.
	SessionModeCookie SessionMode = iota

	// SessionModeJWT issues a signed JWT (HS256) carrying the user claims. The
	// JWT is stored in the session cookie and validated on each request.
	SessionModeJWT
)

// Config configures a single provider integration. Only ClientID and
// ClientSecret are required; every other field has a secure default applied by
// normalize().
type Config struct {
	// Provider selects the identity provider (Google, GitHub, ...).
	Provider Provider

	// ClientID / ClientSecret are the OAuth application credentials. Required.
	ClientID     string
	ClientSecret string

	// RedirectURL is the absolute callback URL registered with the provider.
	// If empty, BaseURL + "/auth/{provider}/callback" is used.
	RedirectURL string

	// BaseURL is the public origin of the app (e.g. "https://app.example.com").
	// Used to synthesize RedirectURL when it is omitted. Defaults to
	// "http://localhost:3000" for local development.
	BaseURL string

	// Scopes overrides the provider's default scopes when non-empty.
	Scopes []string

	// SessionMode selects cookie or JWT session persistence.
	SessionMode SessionMode

	// CookieName is the session cookie name. Defaults to "breeze_oauth_session".
	CookieName string

	// SuccessRedirect is where the user is sent after a successful callback.
	// Defaults to "/".
	SuccessRedirect string

	// FailureRedirect is where the user is sent when authentication fails. When
	// empty, the middleware writes a 401 response instead of redirecting.
	FailureRedirect string

	// CookieSecret signs session and state cookies (HMAC). If empty a process
	// random secret is generated at normalize() time — set this explicitly in
	// production so sessions survive restarts and multiple instances.
	CookieSecret string

	// JWTSecret signs session JWTs when SessionMode == SessionModeJWT. Falls
	// back to CookieSecret when empty.
	JWTSecret string

	// SessionTTL is how long a session/JWT remains valid. Defaults to 24h.
	SessionTTL time.Duration

	// StateTTL bounds how long a login flow (state + PKCE) may take to
	// complete. Defaults to 10 minutes.
	StateTTL time.Duration

	// ClockSkew tolerance applied to token/JWT expiry checks. Defaults to 1m.
	ClockSkew time.Duration

	// Secure forces the "Secure" cookie attribute (HTTPS-only). Defaults to
	// true. Set to false ONLY for local http development.
	Secure bool

	// DisablePKCE turns off PKCE. PKCE is enabled by default; leave it on.
	DisablePKCE bool

	// HTTPClient is reused for all outbound provider calls. When nil a shared,
	// pooled client is used. Exposed for testing (mock servers) and custom
	// transports.
	HTTPClient *http.Client

	// driver caches the resolved ProviderDriver after normalize().
	driver ProviderDriver

	// normalized guards against double normalization.
	normalized bool
}

// sharedHTTPClient is reused across all configs that don't supply their own.
// A single client means connection pooling / keep-alive across every provider
// call in the process.
var sharedHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	},
}

// normalize validates required fields and fills defaults. It is idempotent and
// safe to call multiple times; the middleware calls it once per constructed
// handler. It returns an error only for unrecoverable misconfiguration
// (missing credentials or unknown provider).
func (c *Config) normalize() error {
	if c.normalized {
		return nil
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return ErrMissingCredentials
	}

	drv, ok := lookupDriver(c.Provider)
	if !ok {
		return ErrUnknownProvider
	}
	c.driver = drv

	if len(c.Scopes) == 0 {
		c.Scopes = drv.DefaultScopes()
	}
	if c.BaseURL == "" {
		c.BaseURL = "http://localhost:3000"
	}
	if c.RedirectURL == "" {
		c.RedirectURL = c.BaseURL + "/auth/" + c.Provider.String() + "/callback"
	}
	if c.CookieName == "" {
		c.CookieName = "breeze_oauth_session"
	}
	if c.SuccessRedirect == "" {
		c.SuccessRedirect = "/"
	}
	if c.CookieSecret == "" {
		c.CookieSecret = randomString(32)
	}
	if c.JWTSecret == "" {
		c.JWTSecret = c.CookieSecret
	}
	if c.SessionTTL == 0 {
		c.SessionTTL = 24 * time.Hour
	}
	if c.StateTTL == 0 {
		c.StateTTL = 10 * time.Minute
	}
	if c.ClockSkew == 0 {
		c.ClockSkew = time.Minute
	}
	if c.HTTPClient == nil {
		c.HTTPClient = sharedHTTPClient
	}
	// Secure defaults to true. We can't distinguish "explicitly false" from the
	// zero value on a bool, so callers doing local http must set Secure:false
	// AND BaseURL to an http:// origin. Here we auto-relax Secure when BaseURL
	// is plain http (developer convenience) unless they force it.
	if !c.Secure {
		c.Secure = len(c.BaseURL) >= 8 && c.BaseURL[:8] == "https://"
	}

	c.normalized = true
	return nil
}

// mustNormalize panics with a descriptive error. Middleware constructors call
// this because a misconfigured route is a programming error that should fail
// loudly at startup rather than silently at request time.
func (c *Config) mustNormalize() {
	if err := c.normalize(); err != nil {
		panic("oauth2: " + err.Error())
	}
}

// pkceEnabled reports whether PKCE should be used for this config.
func (c *Config) pkceEnabled() bool { return !c.DisablePKCE }
