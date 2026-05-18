package web

import (
	"net/http"
)

type DashboardData struct {
	Title      string
	Schedules  []ScheduleView
	Containers []ContainerView
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
	Enabled         bool
	ContainerCount int
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	schedules, _ := s.store.ListSchedules()
	containers, _ := s.docker.ListContainers(r.Context())

	tagCache := make(map[string]string)
	tags, _ := s.store.ListTags()
	for _, tag := range tags {
		tagCache[tag.ID] = tag.Name
	}

	scheduleViews := make([]ScheduleView, len(schedules))
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
		scheduleViews[i] = sv
	}

	containerViews := make([]ContainerView, len(containers))
	for i, c := range containers {
		containerViews[i] = ContainerView{
			ID:        c.ID,
			Name:      c.Name,
			Image:     c.Image,
			State:     c.State,
			Status:    c.Status,
			StackName: c.StackName,
		}
	}

	data := DashboardData{
		Title:      "Dashboard",
		Schedules:  scheduleViews,
		Containers: containerViews,
	}

	s.templates["dashboard.html"].ExecuteTemplate(w, "layout", data)
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.docker.ListContainers(r.Context())

	containerViews := make([]ContainerView, len(containers))
	for i, c := range containers {
		containerViews[i] = ContainerView{
			ID:        c.ID,
			Name:      c.Name,
			Image:     c.Image,
			State:     c.State,
			Status:    c.Status,
			StackName: c.StackName,
		}
	}

	data := struct {
		Title      string
		Containers []ContainerView
	}{
		Title:      "Containers",
		Containers: containerViews,
	}

	s.templates["containers.html"].ExecuteTemplate(w, "layout", data)
}

func (s *Server) handleSchedulesNew(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.docker.ListContainers(r.Context())

	containerViews := make([]ContainerView, len(containers))
	for i, c := range containers {
		containerViews[i] = ContainerView{
			ID:   c.ID,
			Name: c.Name,
		}
	}

	data := struct {
		Title      string
		Containers []ContainerView
	}{
		Title:      "New Schedule",
		Containers: containerViews,
	}

	s.templates["schedules.html"].ExecuteTemplate(w, "layout", data)
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

	data := struct {
		Title   string
		Presets []PresetView
	}{
		Title:   "Presets",
		Presets: presetViews,
	}

	s.templates["presets.html"].ExecuteTemplate(w, "layout", data)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, _ := s.store.ListTags()

	tagViews := make([]TagView, len(tags))
	for i, tag := range tags {
		schedules, _ := s.store.ListSchedulesByTag(tag.ID)
		tagViews[i] = TagView{
			ID:             tag.ID,
			Name:           tag.Name,
			StartCron:      tag.StartCron,
			StopCron:       tag.StopCron,
			Enabled:        tag.Enabled,
			ContainerCount: len(schedules),
		}
	}

	data := struct {
		Title string
		Tags  []TagView
	}{
		Title: "Tags",
		Tags:  tagViews,
	}

	s.templates["tags.html"].ExecuteTemplate(w, "layout", data)
}