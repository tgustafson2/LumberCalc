// Package clerkauth verifies Clerk session JWTs and loads display names.
package clerkauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/clerk/clerk-sdk-go/v2/user"

	"lumbercalc/backend/internal/config"
	"lumbercalc/backend/internal/store"
)

// ErrUnauthenticated is a missing or rejected session.
// Callers map it to 401. Any other error from Authenticate is a server failure,
// including a JWKS outage.
var ErrUnauthenticated = errors.New("unauthenticated")

// errUnknownKey is an unknown kid. jwt.GetJSONWebKey has no sentinel for that miss.
var errUnknownKey = errors.New("unknown key")

const (
	keyTTL             = time.Hour
	missWindow         = time.Minute
	displayNameTimeout = 2 * time.Second
)

// Clerk verifies session tokens against an allow list of authorized parties.
type Clerk struct {
	parties []string
	users   *user.Client
	keys    *keyCache
	log     *slog.Logger
}

// New requires a Clerk secret, at least one authorized party, and a logger.
func New(secret config.Secret, authorizedParties []string, log *slog.Logger) (*Clerk, error) {
	if !secret.IsSet() {
		return nil, errors.New("clerk secret is required")
	}
	if len(authorizedParties) == 0 {
		return nil, errors.New("authorized parties are required")
	}
	if log == nil {
		return nil, errors.New("log is required")
	}
	cfg := &clerk.ClientConfig{}
	cfg.Key = clerk.String(secret.Reveal())
	return &Clerk{
		parties: authorizedParties,
		users:   user.NewClient(cfg),
		keys:    newKeyCache(fetchFrom(jwks.NewClient(cfg))),
		log:     log,
	}, nil
}

// Authenticate reads a Bearer token and verifies it with Clerk's JWKS.
// azp must equal a party passed to New. A blank azp is rejected.
// A bad session, including an unknown signing key, returns ErrUnauthenticated.
// A JWKS outage is returned unchanged. Unknown key ids cause at most one JWKS fetch per minute.
// The log records a reason and omits the token.
func (c *Clerk) Authenticate(r *http.Request) (store.ClerkUserID, error) {
	token, ok := bearer(r.Header.Get("Authorization"))
	if !ok {
		c.unauthenticated(r.Context(), "missing")
		return store.ClerkUserID{}, ErrUnauthenticated
	}
	decoded, err := jwt.Decode(r.Context(), &jwt.DecodeParams{Token: token})
	if err != nil || decoded.KeyID == "" {
		c.unauthenticated(r.Context(), "invalid")
		return store.ClerkUserID{}, ErrUnauthenticated
	}
	key, err := c.keys.get(r.Context(), decoded.KeyID)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			c.unauthenticated(r.Context(), "invalid")
			return store.ClerkUserID{}, ErrUnauthenticated
		}
		c.log.ErrorContext(r.Context(), "jwks", "err", err)
		return store.ClerkUserID{}, err
	}
	claims, err := jwt.Verify(r.Context(), &jwt.VerifyParams{
		Token:                  token,
		JWK:                    key,
		AuthorizedPartyHandler: c.allowParty,
	})
	if err != nil {
		c.unauthenticated(r.Context(), "invalid")
		return store.ClerkUserID{}, ErrUnauthenticated
	}
	id, err := store.ParseClerkUserID(claims.Subject)
	if err != nil {
		c.unauthenticated(r.Context(), "subject")
		return store.ClerkUserID{}, ErrUnauthenticated
	}
	return id, nil
}

// DisplayName asks Clerk for the user.
// It returns the full name when both parts are set, otherwise the first name,
// the last name, the username, or the primary email.
// The Clerk call is canceled after 2 seconds.
func (c *Clerk) DisplayName(ctx context.Context, id store.ClerkUserID) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, displayNameTimeout)
	defer cancel()
	u, err := c.users.Get(ctx, id.String())
	if err != nil {
		return "", err
	}
	return formatDisplayName(u), nil
}

func (c *Clerk) unauthenticated(ctx context.Context, reason string) {
	c.log.WarnContext(ctx, "unauthenticated", "reason", reason)
}

// allowParty rejects a blank azp. The SDK helper AuthorizedPartyMatches allows one.
func (c *Clerk) allowParty(azp string) bool {
	if azp == "" {
		return false
	}
	for _, p := range c.parties {
		if azp == p {
			return true
		}
	}
	return false
}

func bearer(header string) (string, bool) {
	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

func formatDisplayName(u *clerk.User) string {
	if u == nil {
		return ""
	}
	first := strings.TrimSpace(deref(u.FirstName))
	last := strings.TrimSpace(deref(u.LastName))
	switch {
	case first != "" && last != "":
		return first + " " + last
	case first != "":
		return first
	case last != "":
		return last
	}
	if name := strings.TrimSpace(deref(u.Username)); name != "" {
		return name
	}
	return primaryEmail(u)
}

func primaryEmail(u *clerk.User) string {
	if u.PrimaryEmailAddressID == nil {
		return ""
	}
	id := *u.PrimaryEmailAddressID
	for _, e := range u.EmailAddresses {
		if e != nil && e.ID == id {
			return strings.TrimSpace(e.EmailAddress)
		}
	}
	return ""
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type keyFetch func(context.Context, string) (*clerk.JSONWebKey, error)

type cachedKey struct {
	key     *clerk.JSONWebKey
	expires time.Time
}

type keyCache struct {
	mu       sync.Mutex
	positive map[string]cachedKey
	missedAt time.Time
	fetch    keyFetch
	now      func() time.Time
}

func newKeyCache(fetch keyFetch) *keyCache {
	return &keyCache{
		positive: map[string]cachedKey{},
		fetch:    fetch,
		now:      time.Now,
	}
}

func (k *keyCache) get(ctx context.Context, kid string) (*clerk.JSONWebKey, error) {
	k.mu.Lock()
	now := k.now()
	if ent, ok := k.positive[kid]; ok && now.Before(ent.expires) {
		key := ent.key
		k.mu.Unlock()
		return key, nil
	}
	// Reserve the window before the fetch. A second kid in that minute must not call Clerk.
	if !k.missedAt.IsZero() && now.Sub(k.missedAt) < missWindow {
		k.mu.Unlock()
		return nil, ErrUnauthenticated
	}
	k.missedAt = now
	k.mu.Unlock()

	key, err := k.fetch(ctx, kid)
	if err != nil {
		if errors.Is(err, errUnknownKey) {
			return nil, ErrUnauthenticated
		}
		k.mu.Lock()
		k.missedAt = time.Time{}
		k.mu.Unlock()
		return nil, err
	}
	k.mu.Lock()
	k.positive[kid] = cachedKey{key: key, expires: now.Add(keyTTL)}
	k.mu.Unlock()
	return key, nil
}

func fetchFrom(client *jwks.Client) keyFetch {
	return func(ctx context.Context, kid string) (*clerk.JSONWebKey, error) {
		key, err := jwt.GetJSONWebKey(ctx, &jwt.GetJSONWebKeyParams{
			KeyID:      kid,
			JWKSClient: client,
		})
		if err != nil {
			if isMissingKey(err) {
				return nil, errUnknownKey
			}
			return nil, err
		}
		if key == nil {
			return nil, errUnknownKey
		}
		return key, nil
	}
}

func isMissingKey(err error) bool {
	switch err.Error() {
	case "missing json web key", "missing jwt kid header claim":
		return true
	default:
		return false
	}
}
