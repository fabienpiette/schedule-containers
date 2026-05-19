package web

import (
	"log/slog"
	"net/http"
	"strings"
)

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func (s *Server) respondNoContent(w http.ResponseWriter, r *http.Request, toastMsg string) {
	if wantsHTML(r) {
		w.Header().Set("X-Toast-Message", toastMsg)
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) renderPartial(w http.ResponseWriter, name string, data any) {
	for _, t := range s.templates {
		if t.Lookup(name) != nil {
			if err := t.ExecuteTemplate(w, name, data); err != nil {
				slog.Error("failed to render partial", "name", name, "error", err)
			}
			return
		}
	}
	slog.Error("partial template not found", "name", name)
	http.Error(w, "template not found", http.StatusInternalServerError)
}
