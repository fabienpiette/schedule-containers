package web

import (
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/ondemand"
	"github.com/gndm/schedule-containers/internal/scheduler"
	"github.com/gndm/schedule-containers/internal/yamlconfig"
)

type applyRequest struct {
	Containers []string `json:"containers"`
}

func (s *Server) apiListContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := s.docker.ListContainers(r.Context())
	if err != nil {
		slog.Error("failed to list containers", "error", err)
		http.Error(w, "failed to list containers", http.StatusInternalServerError)
		return
	}

	if wantsHTML(r) {
		schedules, _ := s.store.ListSchedules(r.Context())
		tags, _ := s.store.ListTags(r.Context())
		stacks, _ := s.store.ListStacks(r.Context())
		tagCache := buildTagCache(tags)
		stackNameSet := buildStackNameSet(stacks)
		tagOptions := make([]TagOption, len(tags))
		for i, t := range tags {
			tagOptions[i] = TagOption{ID: t.ID, Name: t.Name}
		}
		slices.SortFunc(tagOptions, func(a, b TagOption) int { return cmp.Compare(a.Name, b.Name) })
		views := buildContainerViews(containers, schedules, tagCache, stackNameSet)
		slices.SortFunc(views, func(a, b ContainerView) int { return cmp.Compare(a.Name, b.Name) })
		w.Header().Set("Content-Type", "text/html")
		s.renderPartial(w, "container-tbody", ContainersData{
			Containers: views,
			Tags:       tagOptions,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(containers)
}

func (s *Server) apiListSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := s.store.ListSchedules(r.Context())
	if err != nil {
		slog.Error("failed to list schedules", "error", err)
		http.Error(w, "failed to list schedules", http.StatusInternalServerError)
		return
	}

	if wantsHTML(r) {
		tags, _ := s.store.ListTags(r.Context())
		stacks, _ := s.store.ListStacks(r.Context())
		tagCache := buildTagCache(tags)
		stackNameSet := buildStackNameSet(stacks)
		scheduleViews := buildScheduleViews(schedules, tagCache, stackNameSet)
		slices.SortFunc(scheduleViews, func(a, b ScheduleView) int { return cmp.Compare(a.DisplayName, b.DisplayName) })
		w.Header().Set("Content-Type", "text/html")
		s.renderPartial(w, "schedule-tbody", SchedulesData{
			Schedules: scheduleViews,
		})
		return
	}

	type scheduleResponse struct {
		models.Schedule
		TagName string `json:"tag_name,omitempty"`
	}

	ctx := r.Context()
	tagCache := make(map[string]string)
	resp := make([]scheduleResponse, len(schedules))
	for i, sched := range schedules {
		sr := scheduleResponse{Schedule: sched}
		if sched.TagID != nil {
			name, ok := tagCache[*sched.TagID]
			if !ok {
				tag, err := s.store.GetTag(ctx, *sched.TagID)
				if err == nil {
					name = tag.Name
					tagCache[*sched.TagID] = name
				}
			}
			sr.TagName = name
		}
		resp[i] = sr
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

	hasStartCron := req.StartCron != ""
	hasStopCron := req.StopCron != ""

	if !req.OnDemandEnabled && (!hasStartCron || !hasStopCron) {
		http.Error(w, "start_cron and stop_cron are required when on_demand_enabled is false", http.StatusBadRequest)
		return
	}
	if hasStartCron {
		if err := scheduler.ValidateCronExpression(req.StartCron); err != nil {
			http.Error(w, "invalid start_cron: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if hasStopCron {
		if err := scheduler.ValidateCronExpression(req.StopCron); err != nil {
			http.Error(w, "invalid stop_cron: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.OnDemandEnabled {
		if req.OnDemandURL == "" {
			http.Error(w, "on_demand_url is required when on_demand_enabled is true", http.StatusBadRequest)
			return
		}
		if _, err := url.Parse(req.OnDemandURL); err != nil {
			http.Error(w, "invalid on_demand_url: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	slog.Info("creating schedule", "container", req.ContainerName, "stack_name", req.StackName, "tag_id", req.TagID, "on_demand_enabled", req.OnDemandEnabled, "start_cron", req.StartCron, "stop_cron", req.StopCron)
	created, err := s.store.CreateSchedule(r.Context(), &req)
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

	if created.OnDemandEnabled {
		s.ondemand.Watch(created)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) apiUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := s.store.GetSchedule(r.Context(), id)
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
	req.TagID = existing.TagID

	if req.TagID != nil && (req.StartCron != existing.StartCron || req.StopCron != existing.StopCron) {
		http.Error(w, "Cannot edit cron on tag-derived schedule; update the tag instead", http.StatusBadRequest)
		return
	}

	hasStartCron := req.StartCron != ""
	hasStopCron := req.StopCron != ""

	if !req.OnDemandEnabled && (!hasStartCron || !hasStopCron) {
		http.Error(w, "start_cron and stop_cron are required when on_demand_enabled is false", http.StatusBadRequest)
		return
	}
	if hasStartCron {
		if err := scheduler.ValidateCronExpression(req.StartCron); err != nil {
			http.Error(w, "invalid start_cron: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if hasStopCron {
		if err := scheduler.ValidateCronExpression(req.StopCron); err != nil {
			http.Error(w, "invalid stop_cron: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if req.OnDemandEnabled {
		if req.OnDemandURL == "" {
			http.Error(w, "on_demand_url is required when on_demand_enabled is true", http.StatusBadRequest)
			return
		}
		if _, err := url.Parse(req.OnDemandURL); err != nil {
			http.Error(w, "invalid on_demand_url: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	updated, err := s.store.UpdateSchedule(r.Context(), &req)
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

	if existing.OnDemandEnabled {
		s.ondemand.Unwatch(existing.ContainerName)
	}
	if updated.OnDemandEnabled {
		s.ondemand.Watch(updated)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) apiDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	schedule, err := s.store.GetSchedule(r.Context(), id)
	if err != nil {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return
	}

	if err := s.store.DeleteSchedule(r.Context(), id); err != nil {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return
	}

	s.scheduler.RemoveSchedule(id)

	if schedule.OnDemandEnabled {
		s.ondemand.Unwatch(schedule.ContainerName)
	}

	s.respondNoContent(w, r, "Schedule%20deleted")
}

func (s *Server) apiToggleSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	toggled, err := s.store.ToggleSchedule(r.Context(), id)
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

	if wantsHTML(r) {
		tagName := ""
		if toggled.TagID != nil {
			tag, err := s.store.GetTag(r.Context(), *toggled.TagID)
			if err == nil {
				tagName = tag.Name
			}
		}
		var stackScheduleName string
		if toggled.StackName != "" {
			stacks, _ := s.store.ListStacks(r.Context())
			for _, st := range stacks {
				if st.Name == toggled.StackName && st.Enabled {
					stackScheduleName = st.Name
					break
				}
			}
		}
		sv := ScheduleView{
			ID:                toggled.ID,
			ContainerName:    toggled.ContainerName,
			DisplayName:       toggled.DisplayName,
			StackName:         toggled.StackName,
			StackScheduleName: stackScheduleName,
			StartCron:        toggled.StartCron,
			StopCron:         toggled.StopCron,
			Enabled:           toggled.Enabled,
			OnDemandEnabled:   toggled.OnDemandEnabled,
			OnDemandURL:       toggled.OnDemandURL,
			IdleTimeoutSec:    toggled.IdleTimeoutSec,
			StartupDelaySec:   toggled.StartupDelaySec,
			TagName:           tagName,
		}
		w.Header().Set("X-Toast-Message", "Schedule%20toggled")
		s.renderPartial(w, "schedule-row", sv)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toggled)
}

func (s *Server) apiStartContainer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	slog.Info("manual start container", "container", name)
	if err := s.docker.StartContainer(r.Context(), name); err != nil {
		slog.Error("failed to start container", "container", name, "error", err)
		http.Error(w, "failed to start container", http.StatusInternalServerError)
		return
	}
	slog.Info("started container", "container", name)
	if wantsHTML(r) {
		ctr, err := s.docker.GetContainer(r.Context(), name)
		if err != nil {
			slog.Error("failed to get container after start", "container", name, "error", err)
			s.respondNoContent(w, r, "Container%20started")
			return
		}
		cv := s.buildSingleContainerView(r.Context(), ctr)
		w.Header().Set("X-Toast-Message", "Container%20started")
		s.renderPartial(w, "container-row", cv)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiStopContainer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	slog.Info("manual stop container", "container", name)
	if err := s.docker.StopContainer(r.Context(), name); err != nil {
		slog.Error("failed to stop container", "container", name, "error", err)
		http.Error(w, "failed to stop container", http.StatusInternalServerError)
		return
	}
	slog.Info("stopped container", "container", name)
	if wantsHTML(r) {
		ctr, err := s.docker.GetContainer(r.Context(), name)
		if err != nil {
			slog.Error("failed to get container after stop", "container", name, "error", err)
			s.respondNoContent(w, r, "Container%20stopped")
			return
		}
		cv := s.buildSingleContainerView(r.Context(), ctr)
		w.Header().Set("X-Toast-Message", "Container%20stopped")
		s.renderPartial(w, "container-row", cv)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiListPresets(w http.ResponseWriter, r *http.Request) {
	presets := s.presetService.List()

	if wantsHTML(r) {
		views := make([]PresetView, len(presets))
		for i, p := range presets {
			views[i] = PresetView{
				ID:          p.ID,
				Label:       p.Label,
				Expression:  p.Expression,
				Category:    p.Category,
				Description: p.Description,
			}
		}
		w.Header().Set("Content-Type", "text/html")
		s.renderPartial(w, "preset-tbody", PresetsData{Presets: views})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(presets)
}

func (s *Server) apiCreateCustomPreset(w http.ResponseWriter, r *http.Request) {
	var req models.CronPreset
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	req.Expression = strings.TrimSpace(req.Expression)

	if req.Label == "" {
		http.Error(w, "label is required", http.StatusBadRequest)
		return
	}
	if req.Expression == "" {
		http.Error(w, "expression is required", http.StatusBadRequest)
		return
	}

	created, err := s.presetService.Create(req.Label, req.Expression, req.Category, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) apiDeleteCustomPreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.presetService.Delete(id); err != nil {
		http.Error(w, "preset not found", http.StatusNotFound)
		return
	}
	s.respondNoContent(w, r, "Preset%20deleted")
}

func (s *Server) apiImportSchedules(w http.ResponseWriter, r *http.Request) {
	var body []byte
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "failed to read file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		body, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, "failed to read file content", http.StatusBadRequest)
			return
		}
	} else {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
	}

	schedules, tags, stacks, err := yamlconfig.ToSchedulesTagsAndStacks(body)
	if err != nil {
		http.Error(w, "failed to parse YAML: "+err.Error(), http.StatusBadRequest)
		return
	}

	created := 0
	for _, sched := range schedules {
		if _, err := s.store.CreateSchedule(r.Context(), &sched); err != nil {
			slog.Error("failed to import schedule", "container", sched.ContainerName, "error", err)
			continue
		}
		if sched.Enabled {
			if err := s.scheduler.AddSchedule(&sched); err != nil {
				slog.Warn("failed to add imported schedule to cron runner", "error", err)
			}
		}
		created++
	}

	tagsCreated := 0
	for _, tag := range tags {
		if _, err := s.store.CreateTag(r.Context(), &tag); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				slog.Warn("skipping existing tag during import", "name", tag.Name)
				continue
			}
			slog.Error("failed to import tag", "name", tag.Name, "error", err)
			continue
		}
		tagsCreated++
	}

	stacksCreated := 0
	for _, st := range stacks {
		if _, err := s.store.CreateStack(r.Context(), &st); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				slog.Warn("skipping existing stack during import", "name", st.Name)
				continue
			}
			slog.Error("failed to import stack", "name", st.Name, "error", err)
			continue
		}
		if st.Enabled {
			if err := s.scheduler.AddStack(&st); err != nil {
				slog.Warn("failed to add imported stack to cron runner", "error", err)
			}
		}
		if st.OnDemandEnabled {
			s.stackOndemand.AddStack(&st)
		}
		stacksCreated++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"imported": created, "total": len(schedules), "tags_imported": tagsCreated, "tags_total": len(tags), "stacks_imported": stacksCreated, "stacks_total": len(stacks)})
}

func (s *Server) apiExportSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := s.store.ListSchedules(r.Context())
	if err != nil {
		slog.Error("failed to list schedules", "error", err)
		http.Error(w, "failed to list schedules", http.StatusInternalServerError)
		return
	}

	tags, err := s.store.ListTags(r.Context())
	if err != nil {
		slog.Error("failed to list tags", "error", err)
		http.Error(w, "failed to list tags", http.StatusInternalServerError)
		return
	}

	stacks, err := s.store.ListStacks(r.Context())
	if err != nil {
		slog.Error("failed to list stacks", "error", err)
		http.Error(w, "failed to list stacks", http.StatusInternalServerError)
		return
	}

	data := yamlconfig.FromSchedulesTagsAndStacks(schedules, tags, stacks)
	w.Header().Set("Content-Type", "application/yaml")
	w.Write(data)
}

// --- Tag API handlers ---

func (s *Server) apiListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context())
	if err != nil {
		slog.Error("failed to list tags", "error", err)
		http.Error(w, "failed to list tags", http.StatusInternalServerError)
		return
	}

	if wantsHTML(r) {
		containers, _ := s.docker.ListContainers(r.Context())
		containerNames := make([]string, len(containers))
		containerStates := make(map[string]string, len(containers))
		for i, c := range containers {
			containerNames[i] = c.Name
			containerStates[c.Name] = c.State
		}
		tagViews := make([]TagView, len(tags))
		for i, tag := range tags {
			schedules, _ := s.store.ListSchedulesByTag(r.Context(), tag.ID)
			tagContainers := make([]string, len(schedules))
			for j, sched := range schedules {
				tagContainers[j] = sched.ContainerName
			}
			slices.Sort(tagContainers)
			tagViews[i] = TagView{
				ID:             tag.ID,
				Name:           tag.Name,
				StartCron:      tag.StartCron,
				StopCron:       tag.StopCron,
				Enabled:        tag.Enabled,
				ContainerCount: len(schedules),
				Containers:     tagContainers,
			}
		}
		slices.SortFunc(tagViews, func(a, b TagView) int { return cmp.Compare(a.Name, b.Name) })
		w.Header().Set("Content-Type", "text/html")
		s.renderPartial(w, "tag-tbody", TagsData{
			Tags:            tagViews,
			Containers:      containerNames,
			ContainerStates: containerStates,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

func (s *Server) apiCreateTag(w http.ResponseWriter, r *http.Request) {
	var req models.Tag
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
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

	created, err := s.store.CreateTag(r.Context(), &req)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "tag name already exists", http.StatusConflict)
			return
		}
		slog.Error("failed to create tag", "error", err)
		http.Error(w, "failed to create tag", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) apiGetTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := s.store.GetTag(r.Context(), id)
	if err != nil {
		http.Error(w, "tag not found", http.StatusNotFound)
		return
	}
	schedules, _ := s.store.ListSchedulesByTag(r.Context(), id)

	type tagDetail struct {
		models.Tag
		Schedules []models.Schedule `json:"schedules"`
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tagDetail{Tag: *tag, Schedules: schedules})
}

func (s *Server) apiUpdateTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.store.GetTag(r.Context(), id)
	if err != nil {
		http.Error(w, "tag not found", http.StatusNotFound)
		return
	}

	var req models.Tag
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.ID = id
	req.CreatedAt = existing.CreatedAt

	if req.StartCron == "" {
		req.StartCron = existing.StartCron
	}
	if req.StopCron == "" {
		req.StopCron = existing.StopCron
	}

	if err := scheduler.ValidateCronExpression(req.StartCron); err != nil {
		http.Error(w, "invalid start_cron: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := scheduler.ValidateCronExpression(req.StopCron); err != nil {
		http.Error(w, "invalid stop_cron: "+err.Error(), http.StatusBadRequest)
		return
	}

	cronChanged := req.StartCron != existing.StartCron || req.StopCron != existing.StopCron

	updated, err := s.store.UpdateTag(r.Context(), &req)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "tag name already exists", http.StatusConflict)
			return
		}
		slog.Error("failed to update tag", "error", err)
		http.Error(w, "failed to update tag", http.StatusInternalServerError)
		return
	}

	if cronChanged {
		schedules, _ := s.store.ListSchedulesByTag(r.Context(), id)
		for _, sched := range schedules {
			sched.StartCron = updated.StartCron
			sched.StopCron = updated.StopCron
			updatedSched, err := s.store.UpdateSchedule(r.Context(), &sched)
			if err != nil {
				slog.Warn("failed to update tag schedule cron", "schedule_id", sched.ID, "error", err)
				continue
			}
			s.scheduler.RemoveSchedule(sched.ID)
			if updatedSched.Enabled {
				if err := s.scheduler.AddSchedule(updatedSched); err != nil {
					slog.Warn("failed to re-add schedule after tag cron update", "schedule_id", sched.ID, "error", err)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) apiDeleteTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	schedules, _ := s.store.ListSchedulesByTag(r.Context(), id)
	for _, sched := range schedules {
		s.scheduler.RemoveSchedule(sched.ID)
	}

	if err := s.store.DeleteTag(r.Context(), id); err != nil {
		http.Error(w, "tag not found", http.StatusNotFound)
		return
	}

	s.respondNoContent(w, r, "Tag%20deleted")
}

func (s *Server) apiToggleTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := s.store.GetTag(r.Context(), id)
	if err != nil {
		http.Error(w, "tag not found", http.StatusNotFound)
		return
	}

	tag.Enabled = !tag.Enabled
	updated, err := s.store.UpdateTag(r.Context(), tag)
	if err != nil {
		slog.Error("failed to toggle tag", "error", err)
		http.Error(w, "failed to toggle tag", http.StatusInternalServerError)
		return
	}

	schedules, _ := s.store.ListSchedulesByTag(r.Context(), id)
	for _, sched := range schedules {
		sched.Enabled = updated.Enabled
		updatedSched, err := s.store.UpdateSchedule(r.Context(), &sched)
		if err != nil {
			slog.Warn("failed to update schedule enabled state", "schedule_id", sched.ID, "error", err)
			continue
		}
		s.scheduler.RemoveSchedule(sched.ID)
		if updatedSched.Enabled {
			if err := s.scheduler.AddSchedule(updatedSched); err != nil {
				slog.Warn("failed to re-add schedule after tag toggle", "schedule_id", sched.ID, "error", err)
			}
		}
	}

	if wantsHTML(r) {
		tagSchedules, err := s.store.ListSchedulesByTag(r.Context(), updated.ID)
		if err != nil {
			slog.Warn("failed to list schedules for tag partial", "tag_id", updated.ID, "error", err)
		}
		tv := TagView{
			ID:             updated.ID,
			Name:           updated.Name,
			StartCron:      updated.StartCron,
			StopCron:       updated.StopCron,
			Enabled:        updated.Enabled,
			ContainerCount: len(tagSchedules),
		}
		w.Header().Set("X-Toast-Message", "Tag%20toggled")
		s.renderPartial(w, "tag-row", tv)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) apiApplyTagToContainers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := s.store.GetTag(r.Context(), id)
	if err != nil {
		http.Error(w, "tag not found", http.StatusNotFound)
		return
	}

	var req applyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Containers) == 0 {
		http.Error(w, "containers list is required", http.StatusBadRequest)
		return
	}

	slog.Info("applying tag to containers", "tag", tag.Name, "tag_id", tag.ID, "containers", req.Containers)

	tagID := tag.ID
	var created []models.Schedule
	var skipped []string

	for _, containerName := range req.Containers {
		existing, _ := s.store.GetScheduleByTagAndContainer(r.Context(), tagID, containerName)
		if existing != nil {
			slog.Info("skipping container, already has tag schedule", "container", containerName, "tag", tag.Name)
			skipped = append(skipped, containerName)
			continue
		}

		sched := &models.Schedule{
			ContainerName:   containerName,
			DisplayName:     containerName,
			StartCron:       tag.StartCron,
			StopCron:        tag.StopCron,
			Enabled:         tag.Enabled,
			TagID:           &tagID,
			OnDemandEnabled: false,
			OnDemandURL:     "",
			IdleTimeoutSec:  0,
		}
		slog.Info("creating tag-derived schedule", "container", containerName, "tag_id", tagID, "tag_name", tag.Name)
		createdSched, err := s.store.CreateSchedule(r.Context(), sched)
		if err != nil {
			slog.Error("failed to create schedule for container", "container", containerName, "error", err)
			continue
		}
		if createdSched.Enabled {
			if err := s.scheduler.AddSchedule(createdSched); err != nil {
				slog.Warn("failed to add schedule to cron runner", "schedule_id", createdSched.ID, "error", err)
			}
		}
		created = append(created, *createdSched)
	}

	if wantsHTML(r) && len(created) > 0 {
		ctr, err := s.docker.GetContainer(r.Context(), created[0].ContainerName)
		if err != nil {
			slog.Error("failed to get container after tag apply", "container", created[0].ContainerName, "error", err)
			http.Error(w, "failed to get container state", http.StatusInternalServerError)
			return
		}
		cv := ContainerView{
			ID:             ctr.ID,
			Name:           ctr.Name,
			Image:          ctr.Image,
			State:          ctr.State,
			Status:         ctr.Status,
			StackName:      ctr.StackName,
			StackScheduled: s.isStackScheduled(r.Context(), ctr.StackName),
			TagName:        tag.Name,
			TagID:          tag.ID,
		}
		s.renderPartial(w, "container-row", cv)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"created": created,
		"skipped": skipped,
	})
}

func (s *Server) apiRemoveTagFromContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	containerName := chi.URLParam(r, "name")

	sched, err := s.store.GetScheduleByTagAndContainer(r.Context(), id, containerName)
	if err != nil {
		http.Error(w, "no schedule found for this tag and container", http.StatusNotFound)
		return
	}

	if err := s.store.DeleteSchedule(r.Context(), sched.ID); err != nil {
		slog.Error("failed to delete schedule", "error", err)
		http.Error(w, "failed to delete schedule", http.StatusInternalServerError)
		return
	}
	s.scheduler.RemoveSchedule(sched.ID)

	if wantsHTML(r) {
		ctr, err := s.docker.GetContainer(r.Context(), containerName)
		if err != nil {
			slog.Error("failed to get container after tag remove", "container", containerName, "error", err)
			s.respondNoContent(w, r, "Tag%20removed%20from%20container")
			return
		}
		cv := ContainerView{
			ID:             ctr.ID,
			Name:           ctr.Name,
			Image:          ctr.Image,
			State:          ctr.State,
			Status:         ctr.Status,
			StackName:      ctr.StackName,
			StackScheduled: s.isStackScheduled(r.Context(), ctr.StackName),
		}
		w.Header().Set("X-Toast-Message", "Tag%20removed%20from%20container")
		s.renderPartial(w, "container-row", cv)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiContainerHealth(w http.ResponseWriter, r *http.Request) {
	containerName := chi.URLParam(r, "name")

	result, err := s.ondemand.CheckHealth(r.Context(), containerName)
	if err != nil {
		if errors.Is(err, ondemand.ErrScheduleNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		slog.Error("health check failed", "container", containerName, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"healthy": result.Healthy,
		"url":     result.OnDemandURL,
	})
}

func (s *Server) apiWakeURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	schedule, err := s.store.GetSchedule(r.Context(), id)
	if err != nil {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"wake_url": "/wake/" + schedule.ContainerName + "/",
	})
}

// --- Stack API handlers ---

func (s *Server) apiListStacks(w http.ResponseWriter, r *http.Request) {
	stacks, err := s.store.ListStacks(r.Context())
	if err != nil {
		slog.Error("failed to list stacks", "error", err)
		http.Error(w, "failed to list stacks", http.StatusInternalServerError)
		return
	}

	if wantsHTML(r) {
		containers, _ := s.docker.ListContainers(r.Context())
		countByStack := make(map[string]int)
		for _, c := range containers {
			if c.StackName != "" {
				countByStack[c.StackName]++
			}
		}
		stackViews := make([]StackView, len(stacks))
		for i, st := range stacks {
			stackViews[i] = StackView{
				Stack:          st,
				ContainerCount: countByStack[st.Name],
			}
		}
		slices.SortFunc(stackViews, func(a, b StackView) int { return cmp.Compare(a.Name, b.Name) })
		w.Header().Set("Content-Type", "text/html")
		s.renderPartial(w, "stack-tbody", SchedulesData{Stacks: stackViews})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stacks)
}

func (s *Server) apiCreateStack(w http.ResponseWriter, r *http.Request) {
	var req models.Stack
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.StartCron == "" && req.StopCron == "" && !req.OnDemandEnabled {
		http.Error(w, "at least one of start_cron, stop_cron, or on_demand_enabled is required", http.StatusBadRequest)
		return
	}
	if req.StartCron != "" {
		if err := scheduler.ValidateCronExpression(req.StartCron); err != nil {
			http.Error(w, "invalid start_cron: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.StopCron != "" {
		if err := scheduler.ValidateCronExpression(req.StopCron); err != nil {
			http.Error(w, "invalid stop_cron: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.OnDemandEnabled {
		if req.OnDemandURL == "" {
			http.Error(w, "on_demand_url is required when on_demand_enabled", http.StatusBadRequest)
			return
		}
		if req.PrimaryContainer == "" {
			http.Error(w, "primary_container is required when on_demand_enabled", http.StatusBadRequest)
			return
		}
	}

	slog.Info("creating stack", "name", req.Name, "on_demand_enabled", req.OnDemandEnabled, "start_cron", req.StartCron, "stop_cron", req.StopCron, "primary_container", req.PrimaryContainer)
	created, err := s.store.CreateStack(r.Context(), &req)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "stack name already exists", http.StatusConflict)
			return
		}
		slog.Error("failed to create stack", "error", err)
		http.Error(w, "failed to create stack", http.StatusInternalServerError)
		return
	}

	if created.Enabled {
		if err := s.scheduler.AddStack(created); err != nil {
			slog.Warn("failed to register stack cron", "stack", created.Name, "error", err)
		}
	}
	if created.OnDemandEnabled {
		s.stackOndemand.AddStack(created)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) apiGetStack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stack, err := s.store.GetStack(r.Context(), id)
	if err != nil {
		http.Error(w, "stack not found", http.StatusNotFound)
		return
	}

	containers, _ := s.docker.ListContainers(r.Context())
	var stackContainers []models.Container
	for _, c := range containers {
		if c.StackName == stack.Name {
			stackContainers = append(stackContainers, c)
		}
	}

	type stackDetail struct {
		models.Stack
		Containers []models.Container `json:"containers"`
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stackDetail{Stack: *stack, Containers: stackContainers})
}

func (s *Server) apiUpdateStack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.store.GetStack(r.Context(), id)
	if err != nil {
		http.Error(w, "stack not found", http.StatusNotFound)
		return
	}

	var req models.Stack
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.ID = id
	req.CreatedAt = existing.CreatedAt

	if req.StartCron != "" {
		if err := scheduler.ValidateCronExpression(req.StartCron); err != nil {
			http.Error(w, "invalid start_cron: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.StopCron != "" {
		if err := scheduler.ValidateCronExpression(req.StopCron); err != nil {
			http.Error(w, "invalid stop_cron: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.OnDemandEnabled {
		if req.OnDemandURL == "" {
			http.Error(w, "on_demand_url is required when on_demand_enabled", http.StatusBadRequest)
			return
		}
		if req.PrimaryContainer == "" {
			http.Error(w, "primary_container is required when on_demand_enabled", http.StatusBadRequest)
			return
		}
	}

	updated, err := s.store.UpdateStack(r.Context(), &req)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "stack name already exists", http.StatusConflict)
			return
		}
		slog.Error("failed to update stack", "error", err)
		http.Error(w, "failed to update stack", http.StatusInternalServerError)
		return
	}

	s.stackOndemand.RemoveStack(id)
	if err := s.scheduler.UpdateStack(updated); err != nil {
		slog.Warn("failed to update stack cron", "stack", updated.Name, "error", err)
	}
	if updated.OnDemandEnabled && updated.Enabled {
		s.stackOndemand.AddStack(updated)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) apiDeleteStack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.store.GetStack(r.Context(), id); err != nil {
		http.Error(w, "stack not found", http.StatusNotFound)
		return
	}

	s.scheduler.RemoveStack(id)
	s.stackOndemand.RemoveStack(id)

	if err := s.store.DeleteStack(r.Context(), id); err != nil {
		slog.Error("failed to delete stack", "error", err)
		http.Error(w, "failed to delete stack", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiToggleStack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	toggled, err := s.store.ToggleStack(r.Context(), id)
	if err != nil {
		http.Error(w, "stack not found", http.StatusNotFound)
		return
	}

	s.stackOndemand.RemoveStack(id)
	if toggled.Enabled {
		if err := s.scheduler.AddStack(toggled); err != nil {
			slog.Warn("failed to re-register stack cron on toggle", "stack", toggled.Name, "error", err)
		}
		if toggled.OnDemandEnabled {
			s.stackOndemand.AddStack(toggled)
		}
	} else {
		s.scheduler.RemoveStack(id)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toggled)
}