package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/fabienpiette/schedule-containers/internal/auth"
	"github.com/fabienpiette/schedule-containers/internal/models"
	"github.com/fabienpiette/schedule-containers/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	f, err := os.CreateTemp("", "web-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.Close()
	s, err := store.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func testServer(t *testing.T) *Server {
	t.Helper()
	return &Server{store: testStore(t)}
}

func TestFirstRunRedirect_NoUsers_RedirectsToSetup(t *testing.T) {
	srv := testServer(t)
	handler := srv.firstRunRedirect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/schedules", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/setup" {
		t.Errorf("expected /setup, got %q", loc)
	}
}

func TestFirstRunRedirect_UsersExist_PassesThrough(t *testing.T) {
	srv := testServer(t)
	ctx := context.Background()
	_, _ = srv.store.CreateUser(ctx, &models.User{Username: "admin", PasswordHash: "$h", Role: models.RoleAdmin})

	handler := srv.firstRunRedirect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/schedules", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireRole_NoCookie_RedirectsToLogin(t *testing.T) {
	srv := testServer(t)
	mw := srv.requireRole(models.RoleReader)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/schedules", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected /login, got %q", loc)
	}
}

func TestRequireRole_ValidSession_InjectsUser(t *testing.T) {
	srv := testServer(t)
	ctx := context.Background()

	hash, _ := auth.HashPassword("pass")
	user, _ := srv.store.CreateUser(ctx, &models.User{Username: "alice", PasswordHash: hash, Role: models.RoleReader})
	token, _ := auth.GenerateToken()
	now := time.Now().UTC()
	_ = srv.store.CreateSession(ctx, &models.Session{
		Token: token, UserID: user.ID, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	})

	mw := srv.requireRole(models.RoleReader)
	var gotUser *models.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/schedules", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if gotUser == nil || gotUser.Username != "alice" {
		t.Errorf("expected alice in context, got %v", gotUser)
	}
}

func TestRequireRole_InsufficientRole_Returns403(t *testing.T) {
	srv := testServer(t)
	ctx := context.Background()

	hash, _ := auth.HashPassword("pass")
	user, _ := srv.store.CreateUser(ctx, &models.User{Username: "reader", PasswordHash: hash, Role: models.RoleReader})
	token, _ := auth.GenerateToken()
	now := time.Now().UTC()
	_ = srv.store.CreateSession(ctx, &models.Session{
		Token: token, UserID: user.ID, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	})

	mw := srv.requireRole(models.RoleAdmin)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/api/schedules/x", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireRole_UserAlreadyInContext_SkipsDBLookup(t *testing.T) {
	srv := testServer(t)
	preInjected := &models.User{ID: "x", Username: "pre", Role: models.RoleAdmin}

	mw := srv.requireRole(models.RoleAdmin)
	var gotUser *models.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	ctx := context.WithValue(req.Context(), userContextKey, preInjected)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if gotUser == nil || gotUser.Username != "pre" {
		t.Errorf("expected pre-injected user, got %v", gotUser)
	}
}
