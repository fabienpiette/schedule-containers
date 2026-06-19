package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fabienpiette/schedule-containers/internal/config"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("generateCodeVerifier: %v", err)
	}
	if len(v) < 43 || len(v) > 128 {
		t.Errorf("expected 43–128 chars, got %d", len(v))
	}
	v2, _ := generateCodeVerifier()
	if v == v2 {
		t.Error("expected unique verifiers")
	}
}

func TestCodeChallenge_RFC7636Vector(t *testing.T) {
	// RFC 7636 Appendix B test vector
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := codeChallenge(verifier); got != want {
		t.Errorf("codeChallenge = %q, want %q", got, want)
	}
}

func TestOIDCStateCookieRoundtrip(t *testing.T) {
	rr := httptest.NewRecorder()
	if err := setOIDCStateCookie(rr, "mystate42", "myverifier99"); err != nil {
		t.Fatalf("setOIDCStateCookie: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}
	got, err := getOIDCStateCookie(req)
	if err != nil {
		t.Fatalf("getOIDCStateCookie: %v", err)
	}
	if got.State != "mystate42" {
		t.Errorf("State = %q, want mystate42", got.State)
	}
	if got.CodeVerifier != "myverifier99" {
		t.Errorf("CodeVerifier = %q, want myverifier99", got.CodeVerifier)
	}
}

func TestNewOIDCProvider_Discovery(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": "https://pocket-id.example.com/auth",
			"token_endpoint":         "https://pocket-id.example.com/token",
			"userinfo_endpoint":      "https://pocket-id.example.com/userinfo",
		})
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		OIDCIssuer:       mockServer.URL,
		OIDCClientID:     "sc-client",
		OIDCClientSecret: "sc-secret",
		OIDCRedirectURL:  "https://myapp.example.com/auth/oidc/callback",
	}

	provider, err := newOIDCProvider(cfg)
	if err != nil {
		t.Fatalf("newOIDCProvider: %v", err)
	}
	if provider.userInfoURL != "https://pocket-id.example.com/userinfo" {
		t.Errorf("userInfoURL = %q", provider.userInfoURL)
	}
	if provider.oauth2Cfg.ClientID != "sc-client" {
		t.Errorf("ClientID = %q", provider.oauth2Cfg.ClientID)
	}
	if provider.oauth2Cfg.Endpoint.AuthURL != "https://pocket-id.example.com/auth" {
		t.Errorf("AuthURL = %q", provider.oauth2Cfg.Endpoint.AuthURL)
	}
}

func TestNewOIDCProvider_DiscoveryFails(t *testing.T) {
	cfg := &config.Config{
		OIDCIssuer:       "http://127.0.0.1:1",
		OIDCClientID:     "x",
		OIDCClientSecret: "x",
		OIDCRedirectURL:  "https://app.example.com/callback",
	}
	if _, err := newOIDCProvider(cfg); err == nil {
		t.Error("expected error for unreachable issuer, got nil")
	}
}
