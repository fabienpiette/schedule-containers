package web

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/scheduler"
)

func (s *Server) apiListContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := s.docker.ListContainers(r.Context())
	if err != nil {
		slog.Error("failed to list containers", "error", err)
		http.Error(w, "failed to list containers", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(containers)
}

func (s *Server) apiListSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := s.store.ListSchedules()
	if err != nil {
		slog.Error("failed to list schedules", "error", err)
		http.Error(w, "failed to list schedules", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schedules)
}

func (s *Server) apiCreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req models.Schedule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.ContainerName == "" {
		http.Error(w, "container_name is required", http.StatusBadRequest)
		return
	}
	if req.StartCron == "" || req.StopCron == "" {
		http.Error(w, "start_cron and stop_cron are required", http.StatusBadRequest)
		return
	}
	if err := scheduler.ValidateCronExpression(req.StartCron); err != nil {
		http.Error(w, "invalid start_cron: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := scheduler.ValidateCronExpression(req.StopCron); err != nil {
		http.Error(w, "invalid stop_cron: "+err.Error(), http.StatusBadRequest)
		return
	}

	created, err := s.store.CreateSchedule(&req)
	if err != nil {
		slog.Error("failed to create schedule", "error", err)
		http.Error(w, "failed to create schedule", http.StatusInternalServerError)
		return
	}

	if created.Enabled {
		if err := s.scheduler.AddSchedule(created); err != nil {
			slog.Warn("failed to add schedule to cron runner", "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) apiUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := s.store.GetSchedule(id)
	if err != nil {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return
	}

	var req models.Schedule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.ID = id
	req.CreatedAt = existing.CreatedAt

	updated, err := s.store.UpdateSchedule(&req)
	if err != nil {
		slog.Error("failed to update schedule", "error", err)
		http.Error(w, "failed to update schedule", http.StatusInternalServerError)
		return
	}

	s.scheduler.RemoveSchedule(id)
	if updated.Enabled {
		if err := s.scheduler.AddSchedule(updated); err != nil {
			slog.Warn("failed to re-add schedule to cron runner", "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) apiDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.store.DeleteSchedule(id); err != nil {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return
	}

	s.scheduler.RemoveSchedule(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiToggleSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	toggled, err := s.store.ToggleSchedule(id)
	if err != nil {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return
	}

	s.scheduler.RemoveSchedule(id)
	if toggled.Enabled {
		if err := s.scheduler.AddSchedule(toggled); err != nil {
			slog.Warn("failed to re-add schedule to cron runner", "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toggled)
}

func (s *Server) apiStartContainer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.docker.StartContainer(r.Context(), name); err != nil {
		slog.Error("failed to start container", "container", name, "error", err)
		http.Error(w, "failed to start container", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiStopContainer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.docker.StopContainer(r.Context(), name); err != nil {
		slog.Error("failed to stop container", "container", name, "error", err)
		http.Error(w, "failed to stop container", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}