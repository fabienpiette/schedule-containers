package web

import (
	"net/http"

	"github.com/gndm/schedule-containers/internal/cronpresets"
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
	Builtin     bool
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	schedules, _ := s.store.ListSchedules()
	containers, _ := s.docker.ListContainers(r.Context())

	scheduleViews := make([]ScheduleView, len(schedules))
	for i, sched := range schedules {
		scheduleViews[i] = ScheduleView{
			ID:              sched.ID,
			ContainerName:   sched.ContainerName,
			DisplayName:     sched.DisplayName,
			StackName:       sched.StackName,
			StartCron:       sched.StartCron,
			StopCron:        sched.StopCron,
			Enabled:         sched.Enabled,
			OnDemandEnabled: sched.OnDemandEnabled,
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
		}
	}

	data := DashboardData{
		Title:      "Dashboard",
		Schedules:  scheduleViews,
		Containers: containerViews,
	}

	s.templates.ExecuteTemplate(w, "dashboard.html", data)
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

	s.templates.ExecuteTemplate(w, "containers.html", data)
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

	s.templates.ExecuteTemplate(w, "schedules.html", data)
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	builtins := cronpresets.Builtins()
	custom, _ := s.store.ListCustomPresets()

	builtinViews := make([]PresetView, len(builtins))
	for i, p := range builtins {
		builtinViews[i] = PresetView{
			ID:          p.ID,
			Label:       p.Label,
			Expression:  p.Expression,
			Category:    p.Category,
			Description: p.Description,
			Builtin:     p.Builtin,
		}
	}

	customViews := make([]PresetView, len(custom))
	for i, p := range custom {
		customViews[i] = PresetView{
			ID:          p.ID,
			Label:       p.Label,
			Expression:  p.Expression,
			Category:    p.Category,
			Description: p.Description,
			Builtin:     p.Builtin,
		}
	}

	data := struct {
		Title           string
		BuiltinPresets  []PresetView
		CustomPresets   []PresetView
	}{
		Title:          "Presets",
		BuiltinPresets: builtinViews,
		CustomPresets:  customViews,
	}

	s.templates.ExecuteTemplate(w, "presets.html", data)
}