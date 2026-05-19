package web

import (
	"net/http"
)

type DashboardData struct {
	Title      string
	Schedules  []ScheduleView
	Containers []ContainerView
	Tags       []TagView
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
	ID              string
	Name            string
	StartCron       string
	StopCron        string
	Enabled         bool
	ContainerCount  int
	Containers      []string
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	schedules, _ := s.store.ListSchedules()
	containers, _ := s.docker.ListContainers(r.Context())

	tagCache := make(map[string]string)
	tags, _ := s.store.ListTags()
	for _, tag := range tags {
		tagCache[tag.ID] = tag.Name
	}

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

	schedByContainer := make(map[string]string)
	tagIDByContainer := make(map[string]string)
	for _, sched := range schedules {
		if sched.TagID != nil {
			schedByContainer[sched.ContainerName] = tagCache[*sched.TagID]
			tagIDByContainer[sched.ContainerName] = *sched.TagID
		}
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
			OnDemandEnabled:  sched.OnDemandEnabled,
		}
		if sched.TagID != nil {
			sv.TagName = tagCache[*sched.TagID]
		}
		scheduleViews[i] = sv
	}

	scheduledContainers := make(map[string]bool)
	for _, sched := range schedules {
		scheduledContainers[sched.ContainerName] = true
	}

	containerViews := make([]ContainerView, 0, len(containers))
	for _, c := range containers {
		if !scheduledContainers[c.Name] {
			continue
		}
		containerViews = append(containerViews, ContainerView{
			ID:        c.ID,
			Name:      c.Name,
			Image:     c.Image,
			State:     c.State,
			Status:    c.Status,
			StackName: c.StackName,
			TagName:   schedByContainer[c.Name],
			TagID:     tagIDByContainer[c.Name],
		})
	}

	data := DashboardData{
		Title:      "Dashboard",
		Schedules:  scheduleViews,
		Containers: containerViews,
		Tags:       tagViews,
	}

	s.templates["dashboard.html"].ExecuteTemplate(w, "layout", data)
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.docker.ListContainers(r.Context())
	schedules, _ := s.store.ListSchedules()
	tags, _ := s.store.ListTags()

	tagCache := make(map[string]string)
	for _, tag := range tags {
		tagCache[tag.ID] = tag.Name
	}

	schedByContainer := make(map[string]string)
	tagIDByContainer := make(map[string]string)
	for _, sched := range schedules {
		if sched.TagID != nil {
			schedByContainer[sched.ContainerName] = tagCache[*sched.TagID]
			tagIDByContainer[sched.ContainerName] = *sched.TagID
		}
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
			TagName:   schedByContainer[c.Name],
			TagID:     tagIDByContainer[c.Name],
		}
	}

	type tagOption struct {
		ID   string
		Name string
	}
	tagOptions := make([]tagOption, len(tags))
	for i, t := range tags {
		tagOptions[i] = tagOption{ID: t.ID, Name: t.Name}
	}

	data := struct {
		Title      string
		Containers []ContainerView
		Tags       []tagOption
	}{
		Title:      "Containers",
		Containers: containerViews,
		Tags:       tagOptions,
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
	containers, _ := s.docker.ListContainers(r.Context())

	containerNames := make([]string, len(containers))
	for i, c := range containers {
		containerNames[i] = c.Name
	}

	tagViews := make([]TagView, len(tags))
	for i, tag := range tags {
		schedules, _ := s.store.ListSchedulesByTag(tag.ID)
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

	data := struct {
		Title      string
		Tags       []TagView
		Containers []string
	}{
		Title:      "Tags",
		Tags:       tagViews,
		Containers: containerNames,
	}

	s.templates["tags.html"].ExecuteTemplate(w, "layout", data)
}