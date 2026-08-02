package oauth2

import (
	"context"
	"strings"
	"time"
)

// Provider enumerates the built-in OAuth2 identity providers.
type Provider int

const (
	// Google identity platform (OpenID Connect).
	Google Provider = iota
	// GitHub OAuth apps.
	GitHub
	// Microsoft identity platform (Azure AD / MSA, "common" tenant).
	Microsoft
	// Discord OAuth2.
	Discord
)

// String returns the canonical lowercase slug for the provider, used in
// auto-generated routes such as /login/google and /auth/google/callback.
func (p Provider) String() string {
	switch p {
	case Google:
		return "google"
	case GitHub:
		return "github"
	case Microsoft:
		return "microsoft"
	case Discord:
		return "discord"
	default:
		return "unknown"
	}
}

// ParseProvider maps a slug (case-insensitive) back to a Provider. The second
// return value reports whether the slug was recognized.
func ParseProvider(slug string) (Provider, bool) {
	switch strings.ToLower(strings.TrimSpace(slug)) {
	case "google":
		return Google, true
	case "github":
		return GitHub, true
	case "microsoft":
		return Microsoft, true
	case "discord":
		return Discord, true
	default:
		return 0, false
	}
}

// Token is the normalized result of a successful token exchange or refresh.
// It is provider-agnostic so handlers never need to know which provider issued
// it.
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	Scope        string    `json:"scope,omitempty"`
}

// Valid reports whether the token has a non-empty access token that has not
// expired (with the caller applying any clock-skew tolerance separately).
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true // no expiry information: assume valid
	}
	return time.Now().Before(t.Expiry)
}

// User is the normalized identity returned by every provider. Handlers receive
// this identical structure regardless of which provider authenticated the
// request.
type User struct {
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Username string   `json:"username"`
	Avatar   string   `json:"avatar"`
	Provider Provider `json:"provider"`
}

// ProviderDriver is the strategy implemented by every provider. Drivers are
// stateless and safe for concurrent use; per-request data flows through the
// arguments. This is the single extension point: add a new provider by
// implementing this interface and registering it (see registry.go).
type ProviderDriver interface {
	// Provider returns the Provider constant this driver implements.
	Provider() Provider

	// DefaultScopes returns the scopes requested when the developer does not
	// specify any.
	DefaultScopes() []string

	// AuthURL builds the provider's authorization URL for the given request
	// parameters. pkceChallenge may be empty when PKCE is disabled.
	AuthURL(cfg *Config, state, nonce, pkceChallenge string) string

	// Exchange trades an authorization code for a Token. verifier is the PKCE
	// code verifier (empty when PKCE is disabled).
	Exchange(ctx context.Context, cfg *Config, code, verifier string) (*Token, error)

	// UserInfo fetches and normalizes the authenticated user's profile.
	UserInfo(ctx context.Context, cfg *Config, tok *Token) (*User, error)

	// Refresh exchanges a refresh token for a fresh Token.
	Refresh(ctx context.Context, cfg *Config, refreshToken string) (*Token, error)
}

// endpoints groups the three OAuth URLs a provider uses. Kept unexported so
// providers stay fully described inside their own file.
type endpoints struct {
	auth     string
	token    string
	userInfo string
}
