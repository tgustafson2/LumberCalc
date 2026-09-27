package clerkauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
)

const (
	testParty  = "http://localhost:5173"
	testIssuer = "https://clerk.com"
	testSub    = "user_2abc"
)

func TestAuthenticate(t *testing.T) {
	priv := newKey(t)
	c, log := newTestClerk(t, func(_ context.Context, kid string) (*clerk.JSONWebKey, error) {
		if kid != "kid" {
			return nil, errUnknownKey
		}
		return publicJWK(priv, kid), nil
	})

	t.Run("missing Authorization header is unauthenticated", func(t *testing.T) {
		log.Reset()
		_, err := c.Authenticate(httptest.NewRequest(http.MethodGet, "/v1/me", nil))
		if !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(log.String(), "reason=missing") {
			t.Fatalf("log = %s", log.String())
		}
	})

	t.Run("token signed by a different key is unauthenticated", func(t *testing.T) {
		log.Reset()
		token := sign(t, newKey(t), "kid", baseClaims(testParty, testSub))
		_, err := c.Authenticate(reqWithBearer(token))
		if !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(log.String(), token) {
			t.Fatal("log included the token")
		}
		if !strings.Contains(log.String(), "reason=invalid") {
			t.Fatalf("log = %s", log.String())
		}
	})

	t.Run("empty authorized party is unauthenticated", func(t *testing.T) {
		token := sign(t, priv, "kid", baseClaims("", testSub))
		_, err := c.Authenticate(reqWithBearer(token))
		if !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("authorized party outside the allow list is unauthenticated", func(t *testing.T) {
		token := sign(t, priv, "kid", baseClaims("https://evil.example", testSub))
		_, err := c.Authenticate(reqWithBearer(token))
		if !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("matching authorized party returns the token subject", func(t *testing.T) {
		token := sign(t, priv, "kid", baseClaims(testParty, testSub))
		id, err := c.Authenticate(reqWithBearer(token))
		if err != nil {
			t.Fatal(err)
		}
		if id.String() != testSub {
			t.Fatalf("id = %s", id.String())
		}
	})

	t.Run("empty subject is unauthenticated", func(t *testing.T) {
		log.Reset()
		token := sign(t, priv, "kid", baseClaims(testParty, ""))
		_, err := c.Authenticate(reqWithBearer(token))
		if !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(log.String(), "reason=subject") {
			t.Fatalf("log = %s", log.String())
		}
	})
}

func TestJWKSOutageIsAServerErrorNotAnAuthFailure(t *testing.T) {
	want := errors.New("jwks unavailable")
	c, _ := newTestClerk(t, func(context.Context, string) (*clerk.JSONWebKey, error) {
		return nil, want
	})
	token := sign(t, newKey(t), "kid", baseClaims(testParty, testSub))
	_, err := c.Authenticate(reqWithBearer(token))
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, ErrUnauthenticated) {
		t.Fatal("key source error was unauthenticated")
	}
}

func TestUnknownKeyIDFetchesAtMostOnceAMinute(t *testing.T) {
	fetches := map[string]int{}
	c, _ := newTestClerk(t, func(_ context.Context, kid string) (*clerk.JSONWebKey, error) {
		fetches[kid]++
		return nil, errUnknownKey
	})
	for range 2 {
		token := sign(t, newKey(t), "kid-a", baseClaims(testParty, testSub))
		_, err := c.Authenticate(reqWithBearer(token))
		if !errors.Is(err, ErrUnauthenticated) {
			t.Fatal(err)
		}
	}
	if fetches["kid-a"] != 1 {
		t.Fatalf("kid-a fetches = %d, want 1", fetches["kid-a"])
	}
	token := sign(t, newKey(t), "kid-b", baseClaims(testParty, testSub))
	_, err := c.Authenticate(reqWithBearer(token))
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if fetches["kid-b"] != 1 {
		t.Fatalf("kid-b fetches = %d, want 1", fetches["kid-b"])
	}
}

func TestTwoValidSigningKeysBothAuthenticate(t *testing.T) {
	privA := newKey(t)
	privB := newKey(t)
	fetches := map[string]int{}
	c, _ := newTestClerk(t, func(_ context.Context, kid string) (*clerk.JSONWebKey, error) {
		fetches[kid]++
		switch kid {
		case "kid-a":
			return publicJWK(privA, kid), nil
		case "kid-b":
			return publicJWK(privB, kid), nil
		default:
			return nil, errUnknownKey
		}
	})
	tokenA := sign(t, privA, "kid-a", baseClaims(testParty, testSub))
	tokenB := sign(t, privB, "kid-b", baseClaims(testParty, testSub))
	idA, err := c.Authenticate(reqWithBearer(tokenA))
	if err != nil {
		t.Fatal(err)
	}
	idB, err := c.Authenticate(reqWithBearer(tokenB))
	if err != nil {
		t.Fatal(err)
	}
	if idA.String() != testSub || idB.String() != testSub {
		t.Fatalf("ids = %s %s", idA.String(), idB.String())
	}
	if fetches["kid-a"] != 1 || fetches["kid-b"] != 1 {
		t.Fatalf("fetches = %v", fetches)
	}
}

func TestExpiredTokenIsUnauthenticated(t *testing.T) {
	priv := newKey(t)
	c, _ := newTestClerk(t, func(_ context.Context, kid string) (*clerk.JSONWebKey, error) {
		return publicJWK(priv, kid), nil
	})
	claims := baseClaims(testParty, testSub)
	claims["exp"] = time.Now().Add(-2 * time.Minute).Unix()
	token := sign(t, priv, "kid", claims)
	_, err := c.Authenticate(reqWithBearer(token))
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err = %v", err)
	}
}

func TestJWKSOutageDoesNotBlockTheNextFetch(t *testing.T) {
	fetches := 0
	want := errors.New("jwks unavailable")
	c, _ := newTestClerk(t, func(context.Context, string) (*clerk.JSONWebKey, error) {
		fetches++
		return nil, want
	})
	token := sign(t, newKey(t), "kid", baseClaims(testParty, testSub))
	for range 2 {
		_, err := c.Authenticate(reqWithBearer(token))
		if !errors.Is(err, want) {
			t.Fatalf("err = %v", err)
		}
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d, want 2", fetches)
	}
}

func TestACachedSigningKeyIsFetchedAgainAfterAnHour(t *testing.T) {
	priv := newKey(t)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	fetches := 0
	c, _ := newTestClerk(t, func(context.Context, string) (*clerk.JSONWebKey, error) {
		fetches++
		return publicJWK(priv, "kid"), nil
	})
	c.keys.now = func() time.Time { return now }
	token := sign(t, priv, "kid", baseClaims(testParty, testSub))
	if _, err := c.Authenticate(reqWithBearer(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Authenticate(reqWithBearer(token)); err != nil {
		t.Fatal(err)
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1", fetches)
	}
	now = now.Add(keyTTL + time.Second)
	if _, err := c.Authenticate(reqWithBearer(token)); err != nil {
		t.Fatal(err)
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d, want 2", fetches)
	}
}

func TestFormatDisplayName(t *testing.T) {
	emailID := "idn_1"
	cases := []struct {
		name string
		user *clerk.User
		want string
	}{
		{
			name: "first and last",
			user: &clerk.User{
				FirstName: clerk.String(" Ada "),
				LastName:  clerk.String(" Lovelace "),
				Username:  clerk.String("ada"),
			},
			want: "Ada Lovelace",
		},
		{
			name: "username",
			user: &clerk.User{Username: clerk.String(" ada ")},
			want: "ada",
		},
		{
			name: "email",
			user: &clerk.User{
				PrimaryEmailAddressID: &emailID,
				EmailAddresses: []*clerk.EmailAddress{{
					ID:           emailID,
					EmailAddress: " ada@example.com ",
				}},
			},
			want: "ada@example.com",
		},
		{
			name: "empty",
			user: &clerk.User{},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDisplayName(tc.user); got != tc.want {
				t.Fatalf("name = %q, want %q", got, tc.want)
			}
		})
	}
}

func newTestClerk(t *testing.T, fetch keyFetch) (*Clerk, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	c := &Clerk{
		parties: []string{testParty},
		keys:    newKeyCache(fetch),
		log:     slog.New(slog.NewTextHandler(&buf, nil)),
	}
	return c, &buf
}

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func publicJWK(priv *rsa.PrivateKey, kid string) *clerk.JSONWebKey {
	return &clerk.JSONWebKey{
		Key:       priv.Public(),
		KeyID:     kid,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}
}

func baseClaims(azp, sub string) map[string]any {
	return map[string]any{
		"iss": testIssuer,
		"sub": sub,
		"azp": azp,
	}
}

func sign(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	opts := jose.SignerOptions{}
	opts.WithType("JWT")
	if kid != "" {
		opts.WithHeader("kid", kid)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: priv}, &opts)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(claims).CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func reqWithBearer(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}
