package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
		)`)
	if err != nil {
		return err
	}

	var version int
	row := s.db.QueryRow("SELECT MAX(version) FROM schema_version")
	if row.Scan(&version) != nil {
		version = 0
	}
	if version < 1 {
		_, err = s.db.Exec("INSERT OR IGNORE INTO schema_version (version) VALUES (1)")
		if err != nil {
			return err
		}
		version = 1
	}

	if version < 2 {
		_, err = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS tags (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				start_cron TEXT NOT NULL,
				stop_cron TEXT NOT NULL,
				enabled BOOLEAN NOT NULL DEFAULT TRUE,
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL
			);
			ALTER TABLE schedules ADD COLUMN tag_id TEXT;
			CREATE UNIQUE INDEX IF NOT EXISTS idx_schedules_tag_container ON schedules(tag_id, container_name) WHERE tag_id IS NOT NULL;
			UPDATE schema_version SET version = 2 WHERE version = 1;
			INSERT OR IGNORE INTO schema_version (version) VALUES (2);
		`)
		if err != nil {
			return err
		}
	}

	return nil
}

// --- Schedule CRUD ---

func (s *Store) CreateSchedule(ctx context.Context, schedule *models.Schedule) (*models.Schedule, error) {
	now := time.Now().UTC()
	schedule.ID = uuid.New().String()
	schedule.CreatedAt = now
	schedule.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO schedules (id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, tag_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		schedule.ID, schedule.ContainerName, schedule.DisplayName, schedule.StackName,
		schedule.StartCron, schedule.StopCron, schedule.Enabled,
		schedule.OnDemandEnabled, schedule.OnDemandURL, schedule.IdleTimeoutSec,
		schedule.TagID, schedule.CreatedAt, schedule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *Store) GetSchedule(ctx context.Context, id string) (*models.Schedule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, tag_id, created_at, updated_at
		FROM schedules WHERE id = ?`, id)
	return scanSchedule(row)
}

func (s *Store) ListSchedules(ctx context.Context) ([]models.Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, tag_id, created_at, updated_at
		FROM schedules ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schedules []models.Schedule
	for rows.Next() {
		sched, err := scanScheduleFromRows(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, *sched)
	}
	return schedules, rows.Err()
}

func (s *Store) UpdateSchedule(ctx context.Context, schedule *models.Schedule) (*models.Schedule, error) {
	schedule.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE schedules SET container_name=?, display_name=?, stack_name=?, start_cron=?, stop_cron=?, enabled=?, on_demand_enabled=?, on_demand_url=?, idle_timeout_sec=?, tag_id=?, updated_at=?
		WHERE id=?`,
		schedule.ContainerName, schedule.DisplayName, schedule.StackName,
		schedule.StartCron, schedule.StopCron, schedule.Enabled,
		schedule.OnDemandEnabled, schedule.OnDemandURL, schedule.IdleTimeoutSec,
		schedule.TagID, schedule.UpdatedAt, schedule.ID,
	)
	if err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *Store) DeleteSchedule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM schedules WHERE id = ?", id)
	return err
}

func (s *Store) ToggleSchedule(ctx context.Context, id string) (*models.Schedule, error) {
	sched, err := s.GetSchedule(ctx, id)
	if err != nil {
		return nil, err
	}
	sched.Enabled = !sched.Enabled
	return s.UpdateSchedule(ctx, sched)
}

func (s *Store) ListSchedulesByTag(ctx context.Context, tagID string) ([]models.Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, tag_id, created_at, updated_at
		FROM schedules WHERE tag_id = ? ORDER BY created_at`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schedules []models.Schedule
	for rows.Next() {
		sched, err := scanScheduleFromRows(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, *sched)
	}
	return schedules, rows.Err()
}

func (s *Store) GetOnDemandSchedule(ctx context.Context, containerName string) (*models.Schedule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, tag_id, created_at, updated_at
		FROM schedules WHERE container_name = ? AND on_demand_enabled = TRUE`, containerName)
	sched, err := scanSchedule(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get on-demand schedule for container %q: %w", containerName, sql.ErrNoRows)
		}
		return nil, err
	}
	return sched, nil
}

func (s *Store) GetScheduleByTagAndContainer(ctx context.Context, tagID, containerName string) (*models.Schedule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, tag_id, created_at, updated_at
		FROM schedules WHERE tag_id = ? AND container_name = ?`, tagID, containerName)
	return scanSchedule(row)
}

func scanSchedule(row *sql.Row) (*models.Schedule, error) {
	var sched models.Schedule
	var tagID sql.NullString
	err := row.Scan(&sched.ID, &sched.ContainerName, &sched.DisplayName, &sched.StackName,
		&sched.StartCron, &sched.StopCron, &sched.Enabled, &sched.OnDemandEnabled,
		&sched.OnDemandURL, &sched.IdleTimeoutSec, &tagID, &sched.CreatedAt, &sched.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if tagID.Valid {
		sched.TagID = &tagID.String
	}
	return &sched, nil
}

func scanScheduleFromRows(rows *sql.Rows) (*models.Schedule, error) {
	var sched models.Schedule
	var tagID sql.NullString
	err := rows.Scan(&sched.ID, &sched.ContainerName, &sched.DisplayName, &sched.StackName,
		&sched.StartCron, &sched.StopCron, &sched.Enabled, &sched.OnDemandEnabled,
		&sched.OnDemandURL, &sched.IdleTimeoutSec, &tagID, &sched.CreatedAt, &sched.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if tagID.Valid {
		sched.TagID = &tagID.String
	}
	return &sched, nil
}

// --- Tag CRUD ---

func (s *Store) CreateTag(ctx context.Context, tag *models.Tag) (*models.Tag, error) {
	now := time.Now().UTC()
	tag.ID = uuid.New().String()
	tag.CreatedAt = now
	tag.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tags (id, name, start_cron, stop_cron, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tag.ID, tag.Name, tag.StartCron, tag.StopCron, tag.Enabled,
		tag.CreatedAt, tag.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *Store) GetTag(ctx context.Context, id string) (*models.Tag, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, start_cron, stop_cron, enabled, created_at, updated_at
		FROM tags WHERE id = ?`, id)
	return scanTag(row)
}

func (s *Store) GetTagByName(ctx context.Context, name string) (*models.Tag, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, start_cron, stop_cron, enabled, created_at, updated_at
		FROM tags WHERE name = ?`, name)
	return scanTag(row)
}

func (s *Store) ListTags(ctx context.Context) ([]models.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, start_cron, stop_cron, enabled, created_at, updated_at
		FROM tags ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []models.Tag
	for rows.Next() {
		var tag models.Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.StartCron, &tag.StopCron, &tag.Enabled, &tag.CreatedAt, &tag.UpdatedAt); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *Store) UpdateTag(ctx context.Context, tag *models.Tag) (*models.Tag, error) {
	tag.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE tags SET name=?, start_cron=?, stop_cron=?, enabled=?, updated_at=?
		WHERE id=?`,
		tag.Name, tag.StartCron, tag.StopCron, tag.Enabled, tag.UpdatedAt, tag.ID,
	)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *Store) DeleteTag(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM schedules WHERE tag_id = ?", id); err != nil {
		tx.Rollback()
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM tags WHERE id = ?", id); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func scanTag(row *sql.Row) (*models.Tag, error) {
	var tag models.Tag
	err := row.Scan(&tag.ID, &tag.Name, &tag.StartCron, &tag.StopCron, &tag.Enabled, &tag.CreatedAt, &tag.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &tag, nil
}