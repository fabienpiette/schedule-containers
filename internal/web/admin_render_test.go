package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/gndm/schedule-containers/internal/models"
)

func adminTemplate(t *testing.T) *template.Template {
	t.Helper()
	return template.Must(template.New("").ParseFS(embeddedFS,
		"templates/layout.html",
		"templates/partials.html",
		"templates/admin_users.html",
	))
}

// TestUsersRowRendersID guards the Link-OIDC button: it must carry the user's
// real ID in its onclick so the modal can set linkTargetId.
func TestUsersRowRendersID(t *testing.T) {
	user := &models.User{
		ID:        "81521101-bf32-44e6-9abd-e6df399ac19d",
		Username:  "gunnar",
		Role:      models.RoleAdmin,
		CreatedAt: time.Now(),
	}

	var buf bytes.Buffer
	if err := adminTemplate(t).ExecuteTemplate(&buf, "users-row", user); err != nil {
		t.Fatalf("render users-row: %v", err)
	}

	want := "openLinkModal('81521101-bf32-44e6-9abd-e6df399ac19d')"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("rendered row missing %q\n---\n%s\n---", want, buf.String())
	}
}

// TestLinkModalHiddenByDefault reproduces the "target user not found" bug:
// the link-OIDC modal must be hidden on page load. If its default style is
// display:flex (overriding the `hidden` attribute), the modal shows
// immediately, openLinkModal never runs, linkTargetId stays empty, and the
// link POST hits /admin/users//link-oidc.
func TestLinkModalHiddenByDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := adminTemplate(t).ExecuteTemplate(&buf, "content", adminUsersData{}); err != nil {
		t.Fatalf("render content: %v", err)
	}
	out := buf.String()

	const marker = `id="link-oidc-modal"`
	i := strings.Index(out, marker)
	if i < 0 {
		t.Fatalf("link-oidc-modal not found in rendered content")
	}
	openTag := out[i:strings.Index(out[i:], ">")+i]

	if strings.Contains(openTag, "display:flex") {
		t.Errorf("modal is visible by default (display:flex overrides any hidden state): %s", openTag)
	}
	if !strings.Contains(openTag, "display:none") {
		t.Errorf("modal must be hidden by default via display:none: %s", openTag)
	}
}
