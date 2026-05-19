package web

import (
	"context"
	"net/http"

	"github.com/gndm/schedule-containers/internal/models"
)

type DashboardData struct {
	Title          string
	Schedules      []ScheduleView
	Containers     []ContainerView
	Tags           []TagView
	RunningCount   int
	StoppedCount   int
	SchedulesCount int
	TagsCount      int
}

type ScheduleView struct {
	ID              string
	ContainerName   string
	DisplayName     string
	StackName       string
	StartCron       string
	StopCron        string
	Enabled         bool
	OnDemandEnabled bool
	TagName         string
}

type ContainerView struct {
	ID        string
	Name      string
	Image     string
	State     string
	Status    string
	StackName string
	TagName   string
	TagID     string
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
	Title      string
	Containers []ContainerView
	Schedules  []ScheduleView
}

type PresetsData struct {
	Title   string
	Presets []PresetView
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

func buildScheduleViews(schedules []models.Schedule, tagCache map[string]string) []ScheduleView {
	views := make([]ScheduleView, len(schedules))
	for i, sched := range schedules {
		sv := ScheduleView{
			ID:              sched.ID,
			ContainerName:   sched.ContainerName,
			DisplayName:     sched.DisplayName,
			StackName:       sched.StackName,
			StartCron:       sched.StartCron,
			StopCron:        sched.StopCron,
			Enabled:         sched.Enabled,
			OnDemandEnabled: sched.OnDemandEnabled,
		}
		if sched.TagID != nil {
			sv.TagName = tagCache[*sched.TagID]
		}
		views[i] = sv
	}
	return views
}

func buildContainerViews(containers []models.Container, schedules []models.Schedule, tagCache map[string]string) []ContainerView {
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
			ID:        c.ID,
			Name:      c.Name,
			Image:     c.Image,
			State:     c.State,
			Status:    c.Status,
			StackName: c.StackName,
			TagName:   schedByContainer[c.Name],
			TagID:     tagIDByContainer[c.Name],
		}
	}
	return views
}

func (s *Server) buildSingleContainerView(ctx context.Context, ctr *models.Container) ContainerView {
	schedules, _ := s.store.ListSchedules(ctx)
	tags, _ := s.store.ListTags(ctx)
	tagCache := buildTagCache(tags)
	tagName, tagID := "", ""
	for _, sched := range schedules {
		if sched.ContainerName == ctr.Name && sched.TagID != nil {
			tagName = tagCache[*sched.TagID]
			tagID = *sched.TagID
			break
		}
	}
	return ContainerView{
		ID:        ctr.ID,
		Name:      ctr.Name,
		Image:     ctr.Image,
		State:     ctr.State,
		Status:    ctr.Status,
		StackName: ctr.StackName,
		TagName:   tagName,
		TagID:     tagID,
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	schedules, _ := s.store.ListSchedules(r.Context())
	containers, _ := s.docker.ListContainers(r.Context())
	tags, _ := s.store.ListTags(r.Context())

	tagCache := buildTagCache(tags)
	tagViews := make([]TagView, len(tags))
	for i, tag := range tags {
		tagViews[i] = TagView{
			ID:        tag.ID,
			Name:      tag.Name,
			StartCron: tag.StartCron,
			StopCron:  tag.StopCron,
			Enabled:   tag.Enabled,
		}
	}

	allContainers := buildContainerViews(containers, schedules, tagCache)
	scheduledNames := make(map[string]bool, len(schedules))
	for _, sched := range schedules {
		scheduledNames[sched.ContainerName] = true
	}
	scheduled := make([]ContainerView, 0, len(allContainers))
	for _, cv := range allContainers {
		if scheduledNames[cv.Name] {
			scheduled = append(scheduled, cv)
		}
	}

	runningCount, stoppedCount := 0, 0
	for _, c := range scheduled {
		if c.State == "running" {
			runningCount++
		} else {
			stoppedCount++
		}
	}

	s.renderPage(w, "dashboard.html", DashboardData{
		Title:          "Dashboard",
		Schedules:      buildScheduleViews(schedules, tagCache),
		Containers:     scheduled,
		Tags:           tagViews,
		RunningCount:   runningCount,
		StoppedCount:   stoppedCount,
		SchedulesCount: len(schedules),
		TagsCount:      len(tags),
	})
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.docker.ListContainers(r.Context())
	schedules, _ := s.store.ListSchedules(r.Context())
	tags, _ := s.store.ListTags(r.Context())

	tagCache := buildTagCache(tags)
	tagOptions := make([]TagOption, len(tags))
	for i, t := range tags {
		tagOptions[i] = TagOption{ID: t.ID, Name: t.Name}
	}

	s.renderPage(w, "containers.html", ContainersData{
		Title:      "Containers",
		Containers: buildContainerViews(containers, schedules, tagCache),
		Tags:       tagOptions,
	})
}

func (s *Server) handleSchedulesNew(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.docker.ListContainers(r.Context())
	schedules, _ := s.store.ListSchedules(r.Context())
	tags, _ := s.store.ListTags(r.Context())

	tagCache := buildTagCache(tags)
	containerViews := make([]ContainerView, len(containers))
	for i, c := range containers {
		containerViews[i] = ContainerView{ID: c.ID, Name: c.Name}
	}

	s.renderPage(w, "schedules.html", SchedulesData{
		Title:      "Schedules",
		Containers: containerViews,
		Schedules:  buildScheduleViews(schedules, tagCache),
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

	tagViews := make([]TagView, len(tags))
	for i, tag := range tags {
		schedules, _ := s.store.ListSchedulesByTag(r.Context(), tag.ID)
		tagContainers := make([]string, len(schedules))
		for j, sched := range schedules {
			tagContainers[j] = sched.ContainerName
		}
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

	s.renderPage(w, "tags.html", TagsData{
		Title:           "Tags",
		Tags:            tagViews,
		Containers:      containerNames,
		ContainerStates: containerStates,
	})
}
