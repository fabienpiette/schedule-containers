package web

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"github.com/gndm/schedule-containers/internal/models"
	"github.com/gndm/schedule-containers/internal/ondemand"
)



type ScheduleView struct {
	ID               string
	ContainerName    string
	DisplayName      string
	StackName        string
	StackScheduleName string
	StartCron        string
	StopCron         string
	Enabled          bool
	OnDemandEnabled  bool
	OnDemandURL      string
	IdleTimeoutSec   int
	StartupDelaySec  int
	TagName          string
}

type WakeData struct {
	Title         string
	ContainerName string
	OnDemandURL   string
}

type ContainerView struct {
	ID             string
	Name           string
	Image          string
	State          string
	Status         string
	StackName      string
	StackScheduled bool
	TagName        string
	TagID          string
}

type PresetView struct {
	ID          string
	Label       string
	Expression  string
	Category    string
	Description string
}

type TagView struct {
	ID             string
	Name           string
	StartCron      string
	StopCron       string
	Enabled        bool
	ContainerCount int
	Containers     []string
}

type TagOption struct {
	ID   string
	Name string
}

type ContainersData struct {
	Title      string
	Containers []ContainerView
	Tags       []TagOption
}

type SchedulesData struct {
	Title          string
	Containers     []ContainerView
	Schedules      []ScheduleView
	Stacks         []StackView
	Mode           string
	RunningCount   int
	StoppedCount   int
	SchedulesCount int
}

type PresetsData struct {
	Title   string
	Presets []PresetView
}

type StackView struct {
	models.Stack
	ContainerCount int
}

type TagsData struct {
	Title           string
	Tags            []TagView
	Containers      []string
	ContainerStates map[string]string
}

func buildTagCache(tags []models.Tag) map[string]string {
	cache := make(map[string]string, len(tags))
	for _, tag := range tags {
		cache[tag.ID] = tag.Name
	}
	return cache
}

func buildStackNameSet(stacks []models.Stack) map[string]bool {
	set := make(map[string]bool, len(stacks))
	for _, st := range stacks {
		if st.Enabled {
			set[st.Name] = true
		}
	}
	return set
}

func (s *Server) isStackScheduled(ctx context.Context, stackName string) bool {
	if stackName == "" {
		return false
	}
	stacks, err := s.store.ListStacks(ctx)
	if err != nil {
		return false
	}
	for _, st := range stacks {
		if st.Name == stackName && st.Enabled {
			return true
		}
	}
	return false
}

func buildScheduleViews(schedules []models.Schedule, tagCache map[string]string, stackNameSet map[string]bool) []ScheduleView {
	views := make([]ScheduleView, len(schedules))
	for i, sched := range schedules {
		sv := ScheduleView{
			ID:               sched.ID,
			ContainerName:    sched.ContainerName,
			DisplayName:      sched.DisplayName,
			StackName:        sched.StackName,
			StartCron:        sched.StartCron,
			StopCron:         sched.StopCron,
			Enabled:          sched.Enabled,
			OnDemandEnabled:  sched.OnDemandEnabled,
			OnDemandURL:      sched.OnDemandURL,
			IdleTimeoutSec:   sched.IdleTimeoutSec,
			StartupDelaySec:  sched.StartupDelaySec,
		}
		if sched.TagID != nil {
			sv.TagName = tagCache[*sched.TagID]
		}
		if sched.StackName != "" && stackNameSet[sched.StackName] {
			sv.StackScheduleName = sched.StackName
		}
		views[i] = sv
	}
	return views
}

func buildContainerViews(containers []models.Container, schedules []models.Schedule, tagCache map[string]string, stackNameSet map[string]bool) []ContainerView {
	schedByContainer := make(map[string]string)
	tagIDByContainer := make(map[string]string)
	for _, sched := range schedules {
		if sched.TagID != nil {
			schedByContainer[sched.ContainerName] = tagCache[*sched.TagID]
			tagIDByContainer[sched.ContainerName] = *sched.TagID
		}
	}
	views := make([]ContainerView, len(containers))
	for i, c := range containers {
		views[i] = ContainerView{
			ID:             c.ID,
			Name:           c.Name,
			Image:          c.Image,
			State:          c.State,
			Status:         c.Status,
			StackName:      c.StackName,
			StackScheduled: stackNameSet[c.StackName],
			TagName:        schedByContainer[c.Name],
			TagID:          tagIDByContainer[c.Name],
		}
	}
	return views
}

func (s *Server) buildSingleContainerView(ctx context.Context, ctr *models.Container) ContainerView {
	schedules, _ := s.store.ListSchedules(ctx)
	tags, _ := s.store.ListTags(ctx)
	stacks, _ := s.store.ListStacks(ctx)
	tagCache := buildTagCache(tags)
	stackNameSet := buildStackNameSet(stacks)
	tagName, tagID := "", ""
	for _, sched := range schedules {
		if sched.ContainerName == ctr.Name && sched.TagID != nil {
			tagName = tagCache[*sched.TagID]
			tagID = *sched.TagID
			break
		}
	}
	return ContainerView{
		ID:             ctr.ID,
		Name:           ctr.Name,
		Image:          ctr.Image,
		State:          ctr.State,
		Status:         ctr.Status,
		StackName:      ctr.StackName,
		StackScheduled: stackNameSet[ctr.StackName],
		TagName:        tagName,
		TagID:          tagID,
	}
}



func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.docker.ListContainers(r.Context())
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

	s.renderPage(w, "containers.html", ContainersData{
		Title:      "Containers",
		Containers: views,
		Tags:       tagOptions,
	})
}

func (s *Server) handleSchedulesNew(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.docker.ListContainers(r.Context())
	schedules, _ := s.store.ListSchedules(r.Context())
	tags, _ := s.store.ListTags(r.Context())
	stacks, _ := s.store.ListStacks(r.Context())

	tagCache := buildTagCache(tags)
	stackNameSet := buildStackNameSet(stacks)
	containerViews := make([]ContainerView, len(containers))
	for i, c := range containers {
		containerViews[i] = ContainerView{ID: c.ID, Name: c.Name}
	}
	slices.SortFunc(containerViews, func(a, b ContainerView) int { return cmp.Compare(a.Name, b.Name) })

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

	scheduledNames := make(map[string]bool, len(schedules))
	for _, sched := range schedules {
		scheduledNames[sched.ContainerName] = true
	}
	runningCount, stoppedCount := 0, 0
	for _, c := range containers {
		if scheduledNames[c.Name] {
			if c.State == "running" {
				runningCount++
			} else {
				stoppedCount++
			}
		}
	}

	mode := r.URL.Query().Get("mode")

	scheduleViews := buildScheduleViews(schedules, tagCache, stackNameSet)
	slices.SortFunc(scheduleViews, func(a, b ScheduleView) int { return cmp.Compare(a.DisplayName, b.DisplayName) })

	s.renderPage(w, "schedules.html", SchedulesData{
		Title:          "Schedules",
		Containers:     containerViews,
		Schedules:      scheduleViews,
		Stacks:         stackViews,
		Mode:           mode,
		RunningCount:   runningCount,
		StoppedCount:   stoppedCount,
		SchedulesCount: len(schedules) + len(stacks),
	})
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	presets := s.presetService.List()
	presetViews := make([]PresetView, len(presets))
	for i, p := range presets {
		presetViews[i] = PresetView{
			ID:          p.ID,
			Label:       p.Label,
			Expression:  p.Expression,
			Category:    p.Category,
			Description: p.Description,
		}
	}
	s.renderPage(w, "presets.html", PresetsData{
		Title:   "Presets",
		Presets: presetViews,
	})
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, _ := s.store.ListTags(r.Context())
	containers, _ := s.docker.ListContainers(r.Context())

	containerNames := make([]string, len(containers))
	containerStates := make(map[string]string, len(containers))
	for i, c := range containers {
		containerNames[i] = c.Name
		containerStates[c.Name] = c.State
	}
	slices.Sort(containerNames)

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

	s.renderPage(w, "tags.html", TagsData{
		Title:           "Tags",
		Tags:            tagViews,
		Containers:      containerNames,
		ContainerStates: containerStates,
	})
}

func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	containerName := chi.URLParam(r, "name")

	result, err := s.ondemand.WakeContainer(r.Context(), containerName)
	if err != nil {
		if errors.Is(err, ondemand.ErrScheduleNotFound) {
			// Fall through: try as a stack name (e.g. user accessing /wake/{stackName}/).
			result, err = s.stackOndemand.WakeStack(r.Context(), containerName)
			if err != nil {
				if errors.Is(err, ondemand.ErrStackNotFound) {
					http.Error(w, "no on-demand schedule found for "+containerName, http.StatusNotFound)
					return
				}
				slog.Error("wake stack failed", "stack", containerName, "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			slog.Error("wake container failed", "container", containerName, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if result.Running {
		http.Redirect(w, r, result.OnDemandURL, http.StatusFound)
		return
	}

	s.renderStandalone(w, "wake.html", WakeData{
		Title:         "Waking " + containerName,
		ContainerName: containerName,
		OnDemandURL:   result.OnDemandURL,
	})
}

func (s *Server) handleWakeStatus(w http.ResponseWriter, r *http.Request) {
	containerName := chi.URLParam(r, "name")

	result, err := s.ondemand.CheckHealth(r.Context(), containerName)
	if err != nil {
		if errors.Is(err, ondemand.ErrScheduleNotFound) {
			// Fall through: try as a stack name.
			result, err = s.stackOndemand.CheckStackHealth(r.Context(), containerName)
			if err != nil {
				if errors.Is(err, ondemand.ErrStackNotFound) {
					http.Error(w, "no on-demand schedule found for "+containerName, http.StatusNotFound)
					return
				}
				slog.Error("stack health check failed", "stack", containerName, "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			slog.Error("health check failed", "container", containerName, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if r.Header.Get("HX-Request") == "true" || wantsHTML(r) {
		if result.Healthy {
			w.Header().Set("HX-Redirect", result.OnDemandURL)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<span class="log-line">service not ready yet — polling</span>`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"healthy": result.Healthy,
		"url":     result.OnDemandURL,
	})
}

func (s *Server) handleWakeStack(w http.ResponseWriter, r *http.Request) {
	stackName := chi.URLParam(r, "name")

	result, err := s.stackOndemand.WakeStack(r.Context(), stackName)
	if err != nil {
		if errors.Is(err, ondemand.ErrStackNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		slog.Error("wake stack failed", "stack", stackName, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result.Running {
		http.Redirect(w, r, result.OnDemandURL, http.StatusFound)
		return
	}

	s.renderStandalone(w, "wake.html", WakeData{
		Title:         "Waking " + stackName,
		ContainerName: stackName,
		OnDemandURL:   result.OnDemandURL,
	})
}

func (s *Server) handleWakeStackStatus(w http.ResponseWriter, r *http.Request) {
	stackName := chi.URLParam(r, "name")

	result, err := s.stackOndemand.CheckStackHealth(r.Context(), stackName)
	if err != nil {
		if errors.Is(err, ondemand.ErrStackNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		slog.Error("stack health check failed", "stack", stackName, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" || wantsHTML(r) {
		if result.Healthy {
			w.Header().Set("HX-Redirect", result.OnDemandURL)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<span class="log-line">service not ready yet — polling</span>`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"healthy": result.Healthy,
		"url":     result.OnDemandURL,
	})
}
