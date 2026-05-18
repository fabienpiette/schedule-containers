package store

import (
	"database/sql"
	"time"

	"github.com/gndm/schedule-containers/internal/models"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY
		);
		INSERT OR IGNORE INTO schema_version (version) VALUES (1);

		CREATE TABLE IF NOT EXISTS schedules (
			id TEXT PRIMARY KEY,
			container_name TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			stack_name TEXT NOT NULL DEFAULT '',
			start_cron TEXT NOT NULL,
			stop_cron TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			on_demand_enabled BOOLEAN NOT NULL DEFAULT FALSE,
			on_demand_url TEXT NOT NULL DEFAULT '',
			idle_timeout_sec INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);

		CREATE TABLE IF NOT EXISTS custom_presets (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			expression TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT 'Custom',
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`)
	return err
}

func (s *Store) CreateSchedule(schedule *models.Schedule) (*models.Schedule, error) {
	now := time.Now().UTC()
	schedule.ID = uuid.New().String()
	schedule.CreatedAt = now
	schedule.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO schedules (id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		schedule.ID, schedule.ContainerName, schedule.DisplayName, schedule.StackName,
		schedule.StartCron, schedule.StopCron, schedule.Enabled,
		schedule.OnDemandEnabled, schedule.OnDemandURL, schedule.IdleTimeoutSec,
		schedule.CreatedAt, schedule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *Store) GetSchedule(id string) (*models.Schedule, error) {
	row := s.db.QueryRow(`
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, created_at, updated_at
		FROM schedules WHERE id = ?`, id)
	return scanSchedule(row)
}

func (s *Store) ListSchedules() ([]models.Schedule, error) {
	rows, err := s.db.Query(`
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, created_at, updated_at
		FROM schedules ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []models.Schedule
	for rows.Next() {
		var sched models.Schedule
		if err := rows.Scan(&sched.ID, &sched.ContainerName, &sched.DisplayName, &sched.StackName,
			&sched.StartCron, &sched.StopCron, &sched.Enabled, &sched.OnDemandEnabled,
			&sched.OnDemandURL, &sched.IdleTimeoutSec, &sched.CreatedAt, &sched.UpdatedAt); err != nil {
			return nil, err
		}
		schedules = append(schedules, sched)
	}
	return schedules, rows.Err()
}

func (s *Store) UpdateSchedule(schedule *models.Schedule) (*models.Schedule, error) {
	schedule.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(`
		UPDATE schedules SET container_name=?, display_name=?, stack_name=?, start_cron=?, stop_cron=?, enabled=?, on_demand_enabled=?, on_demand_url=?, idle_timeout_sec=?, updated_at=?
		WHERE id=?`,
		schedule.ContainerName, schedule.DisplayName, schedule.StackName,
		schedule.StartCron, schedule.StopCron, schedule.Enabled,
		schedule.OnDemandEnabled, schedule.OnDemandURL, schedule.IdleTimeoutSec,
		schedule.UpdatedAt, schedule.ID,
	)
	if err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *Store) DeleteSchedule(id string) error {
	_, err := s.db.Exec("DELETE FROM schedules WHERE id = ?", id)
	return err
}

func (s *Store) ToggleSchedule(id string) (*models.Schedule, error) {
	sched, err := s.GetSchedule(id)
	if err != nil {
		return nil, err
	}
	sched.Enabled = !sched.Enabled
	return s.UpdateSchedule(sched)
}

func (s *Store) CreateCustomPreset(preset *models.CronPreset) (*models.CronPreset, error) {
	now := time.Now().UTC()
	preset.ID = uuid.New().String()
	preset.CreatedAt = now
	preset.UpdatedAt = now
	preset.Builtin = false

	_, err := s.db.Exec(`
		INSERT INTO custom_presets (id, label, expression, category, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		preset.ID, preset.Label, preset.Expression, preset.Category, preset.Description,
		preset.CreatedAt, preset.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return preset, nil
}

func (s *Store) ListCustomPresets() ([]models.CronPreset, error) {
	rows, err := s.db.Query(`
		SELECT id, label, expression, category, description, created_at, updated_at
		FROM custom_presets ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var presets []models.CronPreset
	for rows.Next() {
		var p models.CronPreset
		if err := rows.Scan(&p.ID, &p.Label, &p.Expression, &p.Category, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Builtin = false
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

func (s *Store) GetCustomPreset(id string) (*models.CronPreset, error) {
	row := s.db.QueryRow(`
		SELECT id, label, expression, category, description, created_at, updated_at
		FROM custom_presets WHERE id = ?`, id)
	var p models.CronPreset
	err := row.Scan(&p.ID, &p.Label, &p.Expression, &p.Category, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Builtin = false
	return &p, nil
}

func (s *Store) DeleteCustomPreset(id string) error {
	_, err := s.db.Exec("DELETE FROM custom_presets WHERE id = ?", id)
	return err
}

func scanSchedule(row *sql.Row) (*models.Schedule, error) {
	var sched models.Schedule
	err := row.Scan(&sched.ID, &sched.ContainerName, &sched.DisplayName, &sched.StackName,
		&sched.StartCron, &sched.StopCron, &sched.Enabled, &sched.OnDemandEnabled,
		&sched.OnDemandURL, &sched.IdleTimeoutSec, &sched.CreatedAt, &sched.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sched, nil
}