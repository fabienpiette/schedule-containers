package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/fabienpiette/schedule-containers/internal/config"
)

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

type oidcProvider struct {
	oauth2Cfg   oauth2.Config
	userInfoURL string
}

func newOIDCProvider(cfg *config.Config) (*oidcProvider, error) {
	disc, err := fetchOIDCDiscovery(cfg.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}
	return &oidcProvider{
		oauth2Cfg: oauth2.Config{
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			RedirectURL:  cfg.OIDCRedirectURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:  disc.AuthorizationEndpoint,
				TokenURL: disc.TokenEndpoint,
			},
			Scopes: []string{"openid", "profile", "email"},
		},
		userInfoURL: disc.UserInfoEndpoint,
	}, nil
}

func fetchOIDCDiscovery(issuer string) (*oidcDiscovery, error) {
	url := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery returned HTTP %d", resp.StatusCode)
	}
	var d oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.UserInfoEndpoint == "" {
		return nil, fmt.Errorf("OIDC discovery document missing required endpoints")
	}
	return &d, nil
}

func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func codeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type oidcStateCookie struct {
	State        string `json:"state"`
	CodeVerifier string `json:"cv"`
}

const oidcStateCookieName = "oidc_state"

func setOIDCStateCookie(w http.ResponseWriter, state, codeVerifier string) error {
	data, err := json.Marshal(oidcStateCookie{State: state, CodeVerifier: codeVerifier})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    base64.RawURLEncoding.EncodeToString(data),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})
	return nil
}

func getOIDCStateCookie(r *http.Request) (*oidcStateCookie, error) {
	cookie, err := r.Cookie(oidcStateCookieName)
	if err != nil {
		return nil, err
	}
	b, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid state cookie encoding: %w", err)
	}
	var s oidcStateCookie
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("invalid state cookie content: %w", err)
	}
	return &s, nil
}

func clearOIDCStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
	})
}

type oidcUserInfo struct {
	Sub               string `json:"sub"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
}

func (p *oidcProvider) fetchUserInfo(ctx context.Context, token *oauth2.Token) (*oidcUserInfo, error) {
	client := p.oauth2Cfg.Client(ctx, token)
	resp, err := client.Get(p.userInfoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo returned HTTP %d", resp.StatusCode)
	}
	var info oidcUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	if info.Sub == "" {
		return nil, fmt.Errorf("userinfo missing required 'sub' claim")
	}
	return &info, nil
}
