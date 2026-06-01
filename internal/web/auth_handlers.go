package web

import (
	"net/http"
	"time"

	"github.com/gndm/schedule-containers/internal/auth"
	"github.com/gndm/schedule-containers/internal/models"
)

type authPageData struct {
	Error string
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.renderStandalone(w, "login.html", authPageData{})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := s.store.GetUserByUsername(r.Context(), username)
	if err != nil || auth.VerifyPassword(user.PasswordHash, password) != nil {
		s.renderStandalone(w, "login.html", authPageData{Error: "Invalid credentials"})
		return
	}

	token, err := auth.GenerateToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	sess := &models.Session{
		Token:     token,
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
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_token"); err == nil {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "session_token", MaxAge: -1, Path: "/"})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	n, _ := s.store.CountUsers(r.Context())
	if n > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		s.renderStandalone(w, "setup.html", authPageData{})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if username == "" || password == "" {
		s.renderStandalone(w, "setup.html", authPageData{Error: "Username and password are required"})
		return
	}
	if password != confirm {
		s.renderStandalone(w, "setup.html", authPageData{Error: "Passwords do not match"})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if _, err := s.store.CreateUser(r.Context(), &models.User{
		Username:     username,
		PasswordHash: hash,
		Role:         models.RoleAdmin,
	}); err != nil {
		s.renderStandalone(w, "setup.html", authPageData{Error: "Username already taken"})
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
