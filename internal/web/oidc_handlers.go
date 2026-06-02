package web

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"github.com/gndm/schedule-containers/internal/auth"
	"github.com/gndm/schedule-containers/internal/models"
)

func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidcProvider == nil {
		http.NotFound(w, r)
		return
	}

	state, err := generateState()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	verifier, err := generateCodeVerifier()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := setOIDCStateCookie(w, state, verifier); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	url := s.oidcProvider.oauth2Cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidcProvider == nil {
		http.NotFound(w, r)
		return
	}

	stateCookie, err := getOIDCStateCookie(r)
	if err != nil {
		slog.Warn("OIDC callback: missing or invalid state cookie", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	clearOIDCStateCookie(w)

	if r.URL.Query().Get("state") != stateCookie.State {
		slog.Warn("OIDC callback: state mismatch")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		slog.Warn("OIDC callback: provider returned error", "error", errParam)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	token, err := s.oidcProvider.oauth2Cfg.Exchange(r.Context(), r.URL.Query().Get("code"),
		oauth2.SetAuthURLParam("code_verifier", stateCookie.CodeVerifier),
	)
	if err != nil {
		slog.Error("OIDC token exchange failed", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	userInfo, err := s.oidcProvider.fetchUserInfo(r.Context(), token)
	if err != nil {
		slog.Error("OIDC userinfo failed", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	user, err := s.resolveOIDCUser(r.Context(), userInfo)
	if err != nil {
		slog.Error("OIDC user resolution failed", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	sessionToken, err := auth.GenerateToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	sess := &models.Session{
		Token:     sessionToken,
		UserID:    user.ID,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}
	if err := s.store.CreateSession(r.Context(), sess); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) resolveOIDCUser(ctx context.Context, info *oidcUserInfo) (*models.User, error) {
	// 1. Returning OIDC user — match by subject
	if user, err := s.store.GetUserByOIDCSubject(ctx, info.Sub); err == nil {
		return user, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Determine username from preferred_username, email, or sub as fallback
	username := info.PreferredUsername
	if username == "" {
		username = info.Email
	}
	if username == "" {
		username = info.Sub
	}

	// 2. Link existing local account by username
	if user, err := s.store.GetUserByUsername(ctx, username); err == nil {
		user.OIDCSubject = info.Sub
		if err := s.store.UpdateUser(ctx, user); err != nil {
			return nil, err
		}
		return user, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// 3. Auto-provision new reader account
	return s.store.CreateUser(ctx, &models.User{
		Username:     username,
		PasswordHash: "",
		Role:         models.RoleReader,
		OIDCSubject:  info.Sub,
	})
}
