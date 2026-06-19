package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fabienpiette/schedule-containers/internal/auth"
	"github.com/fabienpiette/schedule-containers/internal/models"
	"github.com/go-chi/chi/v5"
)

type adminUsersData struct {
	PageBase
	Title     string
	Users     []*models.User
	OIDCUsers []*models.User
	Error     string
}

func oidcOnlyUsers(users []*models.User) []*models.User {
	var result []*models.User
	for _, u := range users {
		if u.OIDCSubject != "" && u.PasswordHash == "" {
			result = append(result, u)
		}
	}
	return result
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, "admin_users.html", adminUsersData{
		PageBase:  PageBase{CurrentUser: UserFromContext(r.Context())},
		Title:     "Users",
		Users:     users,
		OIDCUsers: oidcOnlyUsers(users),
	})
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")
	role := models.Role(r.FormValue("role"))

	if username == "" || password == "" {
		users, _ := s.store.ListUsers(r.Context())
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = s.templates["admin_users.html"].ExecuteTemplate(w, "users-table", adminUsersData{
			Users:     users,
			OIDCUsers: oidcOnlyUsers(users),
			Error:     "Username and password are required",
		})
		return
	}

	if role != models.RoleReader && role != models.RoleWriter && role != models.RoleAdmin {
		role = models.RoleReader
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if _, err := s.store.CreateUser(r.Context(), &models.User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	}); err != nil {
		users, _ := s.store.ListUsers(r.Context())
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = s.templates["admin_users.html"].ExecuteTemplate(w, "users-table", adminUsersData{
			Users:     users,
			OIDCUsers: oidcOnlyUsers(users),
			Error:     "Username already exists",
		})
		return
	}

	users, _ := s.store.ListUsers(r.Context())
	_ = s.templates["admin_users.html"].ExecuteTemplate(w, "users-table", adminUsersData{Users: users})
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	newRole := models.Role(r.FormValue("role"))
	newPassword := r.FormValue("password")

	if newRole != models.RoleReader && newRole != models.RoleWriter && newRole != models.RoleAdmin {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	user, err := s.store.GetUserByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if user.Role == models.RoleAdmin && newRole != models.RoleAdmin {
		n, _ := s.store.CountAdmins(r.Context())
		if n <= 1 {
			http.Error(w, "cannot demote last admin", http.StatusUnprocessableEntity)
			return
		}
	}

	user.Role = newRole
	if newPassword != "" {
		hash, err := auth.HashPassword(newPassword)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		user.PasswordHash = hash
	}

	if err := s.store.UpdateUser(r.Context(), user); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if newPassword != "" {
		_ = s.store.DeleteSessionsByUserID(r.Context(), user.ID)
	}

	_ = s.templates["admin_users.html"].ExecuteTemplate(w, "users-row", user)
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	currentUser := UserFromContext(r.Context())

	if currentUser != nil && currentUser.ID == id {
		http.Error(w, "cannot delete your own account", http.StatusUnprocessableEntity)
		return
	}

	user, err := s.store.GetUserByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if user.Role == models.RoleAdmin {
		n, _ := s.store.CountAdmins(r.Context())
		if n <= 1 {
			http.Error(w, "cannot delete last admin", http.StatusUnprocessableEntity)
			return
		}
	}

	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAdminLinkOIDC(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")

	var body struct {
		OIDCUserID string `json:"oidc_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	currentUser := UserFromContext(r.Context())
	if currentUser != nil && currentUser.ID == body.OIDCUserID {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "cannot use your own account as the OIDC source"})
		return
	}

	if body.OIDCUserID == targetID {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "cannot link an account to itself"})
		return
	}

	source, err := s.store.GetUserByID(r.Context(), body.OIDCUserID)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if source.Role == models.RoleAdmin {
		n, _ := s.store.CountAdmins(r.Context())
		if n <= 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": "cannot delete last admin account"})
			return
		}
	}

	if err := s.store.LinkOIDCAccount(r.Context(), targetID, body.OIDCUserID); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	isSelf := currentUser != nil && currentUser.ID == targetID
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"is_self": isSelf,
	})
}
