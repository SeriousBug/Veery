// Package fakeidp is a small in-process OpenID Connect provider for Veery's
// tests. go-oidc's oidctest serves discovery and the JWKS; this provider adds a
// token endpoint that mints a signed ID token from configurable claims and
// records the PKCE verifier it was sent.
package fakeidp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
)

// Provider is a running fake identity provider. Fields are read at request
// time, so a test can change the claims between logins.
type Provider struct {
	Server   *httptest.Server
	ClientID string

	// Claims are merged into every ID token. iss, aud, exp and nonce are set by
	// the provider and should not be included.
	Claims map[string]any
	// Nonce, when non-empty, is written into the ID token. Tests set it from
	// the authorization URL to satisfy the verifier, or to a wrong value to
	// exercise the mismatch path.
	Nonce string
	// Challenge, when non-empty, is the PKCE code_challenge the token endpoint
	// checks the received verifier against.
	Challenge string

	mu     sync.Mutex
	pkceOK bool

	priv  *rsa.PrivateKey
	keyID string
}

// New starts a fake provider for the given client id. Call Close when done.
func New(clientID string) *Provider {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	p := &Provider{ClientID: clientID, priv: priv, keyID: "k1"}
	oidcSrv := &oidctest.Server{PublicKeys: []oidctest.PublicKey{{
		PublicKey: priv.Public(), KeyID: p.keyID, Algorithm: gooidc.RS256,
	}}}
	mux := http.NewServeMux()
	mux.Handle("/", oidcSrv)
	mux.HandleFunc("/token", p.handleToken)
	p.Server = httptest.NewServer(mux)
	oidcSrv.SetIssuer(p.Server.URL)
	return p
}

// URL is the issuer URL.
func (p *Provider) URL() string { return p.Server.URL }

// Close shuts the provider down.
func (p *Provider) Close() { p.Server.Close() }

// PKCEOK reports whether the last token exchange carried a verifier matching the
// challenge recorded in Challenge.
func (p *Provider) PKCEOK() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pkceOK
}

func (p *Provider) handleToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if v := r.Form.Get("code_verifier"); v != "" && p.Challenge != "" {
		sum := sha256.Sum256([]byte(v))
		ok := base64.RawURLEncoding.EncodeToString(sum[:]) == p.Challenge
		p.mu.Lock()
		p.pkceOK = ok
		p.mu.Unlock()
	}
	claims := map[string]any{}
	for k, v := range p.Claims {
		claims[k] = v
	}
	claims["iss"] = p.Server.URL
	claims["aud"] = p.ClientID
	claims["exp"] = time.Now().Add(time.Hour).Unix()
	if p.Nonce != "" {
		claims["nonce"] = p.Nonce
	}
	raw, _ := json.Marshal(claims)
	token := oidctest.SignIDToken(p.priv, p.keyID, gooidc.RS256, string(raw))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "at", "token_type": "Bearer", "id_token": token, "expires_in": 3600,
	})
}
