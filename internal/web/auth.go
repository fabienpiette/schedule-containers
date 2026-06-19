package web

import (
	"context"
	"net/http"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

type contextKey int

const userContextKey contextKey = iota

func UserFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(userContextKey).(*models.User)
	return u
}

func (s *Server) firstRunRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := s.store.CountUsers(r.Context())
		if err != nil || n == 0 {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireRole(min models.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())

			if user == nil {
				cookie, err := r.Cookie("session_token")
				if err != nil {
					s.denyAccess(w, r, http.StatusUnauthorized)
					return
				}
				sess, u, err := s.store.GetSessionWithUser(r.Context(), cookie.Value)
				if err != nil || sess.ExpiresAt.Before(time.Now()) {
					http.SetCookie(w, &http.Cookie{Name: "session_token", MaxAge: -1, Path: "/", Secure: true, HttpOnly: true})
					s.denyAccess(w, r, http.StatusUnauthorized)
					return
				}
				user = u
				ctx := context.WithValue(r.Context(), userContextKey, user)
				r = r.WithContext(ctx)
			}

			if !user.Role.AtLeast(min) {
				s.denyAccess(w, r, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) denyAccess(w http.ResponseWriter, r *http.Request, status int) {
	if wantsHTML(r) {
		if status == http.StatusUnauthorized {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		} else {
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
		return
	}
	if status == http.StatusUnauthorized {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	} else {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	}
}
