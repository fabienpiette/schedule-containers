package cronpresets

import (
	"embed"
	"fmt"
	"os"
	"sync"

	"github.com/fabienpiette/schedule-containers/internal/models"
	"github.com/fabienpiette/schedule-containers/internal/scheduler"
	"gopkg.in/yaml.v3"
)

//go:embed presets.yaml
var defaultPresetsFS embed.FS

type PresetEntry struct {
	Label       string `yaml:"label"`
	Expression  string `yaml:"expression"`
	Category    string `yaml:"category"`
	Description string `yaml:"description"`
}

type PresetFile struct {
	Presets []PresetEntry `yaml:"presets"`
}

type Service struct {
	mu       sync.Mutex
	path     string
	presets  []models.CronPreset
	embedded bool
}

func NewService(path string) (*Service, error) {
	s := &Service{path: path}
	if path == "" {
		data, err := defaultPresetsFS.ReadFile("presets.yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded presets: %w", err)
		}
		if err := s.loadFromBytes(data); err != nil {
			return nil, fmt.Errorf("failed to parse embedded presets: %w", err)
		}
		s.embedded = true
		return s, nil
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		data, err := defaultPresetsFS.ReadFile("presets.yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded presets: %w", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, fmt.Errorf("failed to write presets file: %w", err)
		}
	}

	if err := s.loadFromFile(); err != nil {
		return nil, fmt.Errorf("failed to load presets from %s: %w", path, err)
	}
	return s, nil
}

func (s *Service) loadFromBytes(data []byte) error {
	var pf PresetFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return err
	}
	presets := make([]models.CronPreset, len(pf.Presets))
	for i, e := range pf.Presets {
		presets[i] = models.CronPreset{
			ID:          fmt.Sprintf("preset-%d", i+1),
			Label:       e.Label,
			Expression:  e.Expression,
			Category:    e.Category,
			Description: e.Description,
		}
	}
	s.presets = presets
	return nil
}

func (s *Service) loadFromFile() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return s.loadFromBytes(data)
}

func (s *Service) saveToFile() error {
	entries := make([]PresetEntry, len(s.presets))
	for i, p := range s.presets {
		entries[i] = PresetEntry{
			Label:       p.Label,
			Expression:  p.Expression,
			Category:    p.Category,
			Description: p.Description,
		}
	}
	data, err := yaml.Marshal(&PresetFile{Presets: entries})
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Service) List() []models.CronPreset {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]models.CronPreset, len(s.presets))
	copy(result, s.presets)
	return result
}

func (s *Service) Create(label, expression, category, description string) (*models.CronPreset, error) {
	if err := scheduler.ValidateCronExpression(expression); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	if label == "" {
		return nil, fmt.Errorf("label is required")
	}
	if category == "" {
		category = "Custom"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	preset := models.CronPreset{
		ID:          fmt.Sprintf("preset-%d", len(s.presets)+1),
		Label:       label,
		Expression:  expression,
		Category:    category,
		Description: description,
	}
	s.presets = append(s.presets, preset)

	if s.embedded {
		return &preset, nil
	}
	if err := s.saveToFile(); err != nil {
		s.presets = s.presets[:len(s.presets)-1]
		return nil, err
	}
	return &preset, nil
}

func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, p := range s.presets {
		if p.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("preset not found")
	}

	s.presets = append(s.presets[:idx], s.presets[idx+1:]...)

	if s.embedded {
		return nil
	}
	return s.saveToFile()
}
