package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/gndm/schedule-containers/internal/models"
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

	if version < 3 {
		_, err = s.db.Exec(`
			ALTER TABLE schedules ADD COLUMN startup_delay_sec INTEGER NOT NULL DEFAULT 0;
			UPDATE schema_version SET version = 3;
			INSERT OR IGNORE INTO schema_version (version) VALUES (3);
		`)
		if err != nil {
			return err
		}
	}

	if version < 4 {
		_, err = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS stacks (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				display_name TEXT NOT NULL DEFAULT '',
				start_cron TEXT NOT NULL DEFAULT '',
				stop_cron TEXT NOT NULL DEFAULT '',
				enabled BOOLEAN NOT NULL DEFAULT TRUE,
				on_demand_enabled BOOLEAN NOT NULL DEFAULT FALSE,
				on_demand_url TEXT NOT NULL DEFAULT '',
				primary_container TEXT NOT NULL DEFAULT '',
				idle_timeout_sec INTEGER NOT NULL DEFAULT 0,
				startup_delay_sec INTEGER NOT NULL DEFAULT 0,
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL
			);
			UPDATE schema_version SET version = 4;
			INSERT OR IGNORE INTO schema_version (version) VALUES (4);
		`)
		if err != nil {
			return err
		}
	}

	if version < 5 {
		_, err = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS users (
				id            TEXT PRIMARY KEY,
				username      TEXT NOT NULL UNIQUE,
				password_hash TEXT NOT NULL DEFAULT '',
				role          TEXT NOT NULL DEFAULT 'reader',
				oidc_subject  TEXT UNIQUE,
				created_at    TIMESTAMP NOT NULL,
				updated_at    TIMESTAMP NOT NULL
			);
			CREATE TABLE IF NOT EXISTS sessions (
				token      TEXT PRIMARY KEY,
				user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				expires_at TIMESTAMP NOT NULL,
				created_at TIMESTAMP NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
			CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
			UPDATE schema_version SET version = 5;
			INSERT OR IGNORE INTO schema_version (version) VALUES (5);
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
	slog.Info("store: creating schedule", "id", schedule.ID, "container", schedule.ContainerName, "stack_name", schedule.StackName, "tag_id", schedule.TagID, "start_cron", schedule.StartCron, "stop_cron", schedule.StopCron, "on_demand_enabled", schedule.OnDemandEnabled)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO schedules (id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, startup_delay_sec, tag_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		schedule.ID, schedule.ContainerName, schedule.DisplayName, schedule.StackName,
		schedule.StartCron, schedule.StopCron, schedule.Enabled,
		schedule.OnDemandEnabled, schedule.OnDemandURL, schedule.IdleTimeoutSec, schedule.StartupDelaySec,
		schedule.TagID, schedule.CreatedAt, schedule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *Store) GetSchedule(ctx context.Context, id string) (*models.Schedule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, startup_delay_sec, tag_id, created_at, updated_at
		FROM schedules WHERE id = ?`, id)
	return scanSchedule(row)
}

func (s *Store) ListSchedules(ctx context.Context) ([]models.Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, startup_delay_sec, tag_id, created_at, updated_at
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
		UPDATE schedules SET container_name=?, display_name=?, stack_name=?, start_cron=?, stop_cron=?, enabled=?, on_demand_enabled=?, on_demand_url=?, idle_timeout_sec=?, startup_delay_sec=?, tag_id=?, updated_at=?
		WHERE id=?`,
		schedule.ContainerName, schedule.DisplayName, schedule.StackName,
		schedule.StartCron, schedule.StopCron, schedule.Enabled,
		schedule.OnDemandEnabled, schedule.OnDemandURL, schedule.IdleTimeoutSec, schedule.StartupDelaySec,
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
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, startup_delay_sec, tag_id, created_at, updated_at
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
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, startup_delay_sec, tag_id, created_at, updated_at
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
		SELECT id, container_name, display_name, stack_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, idle_timeout_sec, startup_delay_sec, tag_id, created_at, updated_at
		FROM schedules WHERE tag_id = ? AND container_name = ?`, tagID, containerName)
	return scanSchedule(row)
}

func scanSchedule(row *sql.Row) (*models.Schedule, error) {
	var sched models.Schedule
	var tagID sql.NullString
	err := row.Scan(&sched.ID, &sched.ContainerName, &sched.DisplayName, &sched.StackName,
		&sched.StartCron, &sched.StopCron, &sched.Enabled, &sched.OnDemandEnabled,
		&sched.OnDemandURL, &sched.IdleTimeoutSec, &sched.StartupDelaySec, &tagID, &sched.CreatedAt, &sched.UpdatedAt)
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
		&sched.OnDemandURL, &sched.IdleTimeoutSec, &sched.StartupDelaySec, &tagID, &sched.CreatedAt, &sched.UpdatedAt)
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

// --- Stack CRUD ---

func (s *Store) CreateStack(ctx context.Context, stack *models.Stack) (*models.Stack, error) {
	now := time.Now().UTC()
	stack.ID = uuid.New().String()
	stack.CreatedAt = now
	stack.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stacks (id, name, display_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, primary_container, idle_timeout_sec, startup_delay_sec, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		stack.ID, stack.Name, stack.DisplayName, stack.StartCron, stack.StopCron,
		stack.Enabled, stack.OnDemandEnabled, stack.OnDemandURL, stack.PrimaryContainer,
		stack.IdleTimeoutSec, stack.StartupDelaySec, stack.CreatedAt, stack.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return stack, nil
}

func (s *Store) GetStack(ctx context.Context, id string) (*models.Stack, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, display_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, primary_container, idle_timeout_sec, startup_delay_sec, created_at, updated_at
		FROM stacks WHERE id = ?`, id)
	return scanStack(row)
}

func (s *Store) GetStackByName(ctx context.Context, name string) (*models.Stack, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, display_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, primary_container, idle_timeout_sec, startup_delay_sec, created_at, updated_at
		FROM stacks WHERE name = ?`, name)
	return scanStack(row)
}

func (s *Store) ListStacks(ctx context.Context) ([]models.Stack, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, display_name, start_cron, stop_cron, enabled, on_demand_enabled, on_demand_url, primary_container, idle_timeout_sec, startup_delay_sec, created_at, updated_at
		FROM stacks ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stacks []models.Stack
	for rows.Next() {
		var st models.Stack
		if err := rows.Scan(&st.ID, &st.Name, &st.DisplayName, &st.StartCron, &st.StopCron,
			&st.Enabled, &st.OnDemandEnabled, &st.OnDemandURL, &st.PrimaryContainer,
			&st.IdleTimeoutSec, &st.StartupDelaySec, &st.CreatedAt, &st.UpdatedAt); err != nil {
			return nil, err
		}
		stacks = append(stacks, st)
	}
	return stacks, rows.Err()
}

func (s *Store) UpdateStack(ctx context.Context, stack *models.Stack) (*models.Stack, error) {
	stack.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE stacks SET name=?, display_name=?, start_cron=?, stop_cron=?, enabled=?, on_demand_enabled=?, on_demand_url=?, primary_container=?, idle_timeout_sec=?, startup_delay_sec=?, updated_at=?
		WHERE id=?`,
		stack.Name, stack.DisplayName, stack.StartCron, stack.StopCron,
		stack.Enabled, stack.OnDemandEnabled, stack.OnDemandURL, stack.PrimaryContainer,
		stack.IdleTimeoutSec, stack.StartupDelaySec, stack.UpdatedAt, stack.ID,
	)
	if err != nil {
		return nil, err
	}
	return stack, nil
}

func (s *Store) DeleteStack(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM stacks WHERE id = ?", id)
	return err
}

func (s *Store) ToggleStack(ctx context.Context, id string) (*models.Stack, error) {
	stack, err := s.GetStack(ctx, id)
	if err != nil {
		return nil, err
	}
	stack.Enabled = !stack.Enabled
	return s.UpdateStack(ctx, stack)
}

func scanStack(row *sql.Row) (*models.Stack, error) {
	var st models.Stack
	err := row.Scan(&st.ID, &st.Name, &st.DisplayName, &st.StartCron, &st.StopCron,
		&st.Enabled, &st.OnDemandEnabled, &st.OnDemandURL, &st.PrimaryContainer,
		&st.IdleTimeoutSec, &st.StartupDelaySec, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// --- User CRUD ---

const userColumns = `id, username, password_hash, role, oidc_subject, created_at, updated_at`

func scanUser(row *sql.Row) (*models.User, error) {
	u := &models.User{}
	var role string
	var oidcSubj sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &oidcSubj, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.Role = models.Role(role)
	u.OIDCSubject = oidcSubj.String
	return u, nil
}

func scanUserFromRows(rows *sql.Rows) (*models.User, error) {
	u := &models.User{}
	var role string
	var oidcSubj sql.NullString
	if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &oidcSubj, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.Role = models.Role(role)
	u.OIDCSubject = oidcSubj.String
	return u, nil
}

func (s *Store) CreateUser(ctx context.Context, u *models.User) (*models.User, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	oidcSubj := sql.NullString{String: u.OIDCSubject, Valid: u.OIDCSubject != ""}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, role, oidc_subject, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, u.Username, u.PasswordHash, string(u.Role), oidcSubj, now, now,
	)
	if err != nil {
		return nil, err
	}
	result := *u
	result.ID = id
	result.CreatedAt = now
	result.UpdatedAt = now
	return &result, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE username = ?`, username))
}

func (s *Store) GetUserByOIDCSubject(ctx context.Context, subject string) (*models.User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE oidc_subject = ?`, subject))
}

func (s *Store) ListUsers(ctx context.Context) ([]*models.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+userColumns+` FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*models.User
	for rows.Next() {
		u, err := scanUserFromRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, u *models.User) error {
	oidcSubj := sql.NullString{String: u.OIDCSubject, Valid: u.OIDCSubject != ""}
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET username = ?, password_hash = ?, role = ?, oidc_subject = ?, updated_at = ?
		 WHERE id = ?`,
		u.Username, u.PasswordHash, string(u.Role), oidcSubj, time.Now().UTC(), u.ID,
	)
	return err
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	return n, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	return n, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&n)
}

// --- Session CRUD ---

func (s *Store) CreateSession(ctx context.Context, sess *models.Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		sess.Token, sess.UserID, sess.ExpiresAt, sess.CreatedAt,
	)
	return err
}

func (s *Store) GetSessionWithUser(ctx context.Context, token string) (*models.Session, *models.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT s.token, s.user_id, s.expires_at, s.created_at,
		       u.id, u.username, u.password_hash, u.role, u.oidc_subject, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.token = ?`, token)

	sess := &models.Session{}
	u := &models.User{}
	var role string
	var oidcSubj sql.NullString
	err := row.Scan(
		&sess.Token, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt,
		&u.ID, &u.Username, &u.PasswordHash, &role, &oidcSubj, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, nil, err
	}
	u.Role = models.Role(role)
	u.OIDCSubject = oidcSubj.String
	return sess, u, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC())
	return err
}

func (s *Store) DeleteSessionsByUserID(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}