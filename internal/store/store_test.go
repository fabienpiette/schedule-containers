package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/gndm/schedule-containers/internal/models"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp("", "schedule-containers-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.Close()

	s, err := Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// --- Schedule tests ---

func TestOpenAndMigrate(t *testing.T) {
	s := tempDB(t)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestCreateAndGetSchedule(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "My App",
		StackName:     "webstack",
		StartCron:     "0 8 * * 1-5",
		StopCron:      "0 18 * * 1-5",
		Enabled:       true,
	}
	created, err := s.CreateSchedule(context.Background(), sched)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}

	got, err := s.GetSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ContainerName != "my-app" {
		t.Errorf("expected my-app, got %s", got.ContainerName)
	}
	if got.StartCron != "0 8 * * 1-5" {
		t.Errorf("unexpected StartCron: %s", got.StartCron)
	}
}

func TestListSchedules(t *testing.T) {
	s := tempDB(t)
	s1 := &models.Schedule{ContainerName: "app1", DisplayName: "app1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	s2 := &models.Schedule{ContainerName: "app2", DisplayName: "app2", StartCron: "0 9 * * *", StopCron: "0 19 * * *", Enabled: false}
	s.CreateSchedule(context.Background(), s1)
	s.CreateSchedule(context.Background(), s2)

	schedules, err := s.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(schedules) != 2 {
		t.Errorf("expected 2 schedules, got %d", len(schedules))
	}
}

func TestUpdateSchedule(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{ContainerName: "app1", DisplayName: "app1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateSchedule(context.Background(), sched)

	created.StartCron = "0 9 * * *"
	updated, err := s.UpdateSchedule(context.Background(), created)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.StartCron != "0 9 * * *" {
		t.Errorf("expected updated StartCron, got %s", updated.StartCron)
	}
}

func TestDeleteSchedule(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{ContainerName: "app1", DisplayName: "app1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateSchedule(context.Background(), sched)

	err := s.DeleteSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err = s.GetSchedule(context.Background(), created.ID)
	if err == nil {
		t.Error("expected error getting deleted schedule")
	}
}

func TestToggleSchedule(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{ContainerName: "app1", DisplayName: "app1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateSchedule(context.Background(), sched)

	toggled, err := s.ToggleSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if toggled.Enabled != false {
		t.Errorf("expected Enabled=false, got %v", toggled.Enabled)
	}

	toggled2, err := s.ToggleSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if toggled2.Enabled != true {
		t.Errorf("expected Enabled=true, got %v", toggled2.Enabled)
	}
}

func TestScheduleWithTagID(t *testing.T) {
	s := tempDB(t)
	tag, _ := s.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	tagID := tag.ID
	sched := &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	}
	created, err := s.CreateSchedule(context.Background(), sched)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.TagID == nil || *created.TagID != tagID {
		t.Errorf("expected TagID=%s, got %v", tagID, created.TagID)
	}

	got, _ := s.GetSchedule(context.Background(), created.ID)
	if got.TagID == nil || *got.TagID != tagID {
		t.Errorf("expected TagID=%s on read, got %v", tagID, got.TagID)
	}
}

func TestScheduleWithoutTagID(t *testing.T) {
	s := tempDB(t)
	sched := &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	}
	created, err := s.CreateSchedule(context.Background(), sched)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.TagID != nil {
		t.Errorf("expected nil TagID, got %v", created.TagID)
	}

	got, _ := s.GetSchedule(context.Background(), created.ID)
	if got.TagID != nil {
		t.Errorf("expected nil TagID on read, got %v", got.TagID)
	}
}

// --- Tag tests ---

func TestCreateTag(t *testing.T) {
	s := tempDB(t)
	tag := &models.Tag{
		Name:      "business-hours",
		StartCron: "0 8 * * 1-5",
		StopCron:  "0 18 * * 1-5",
		Enabled:   true,
	}
	created, err := s.CreateTag(context.Background(), tag)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Name != "business-hours" {
		t.Errorf("expected name 'business-hours', got %s", created.Name)
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestGetTag(t *testing.T) {
	s := tempDB(t)
	tag := &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateTag(context.Background(), tag)

	got, err := s.GetTag(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.Name != "test" {
		t.Errorf("expected name 'test', got %s", got.Name)
	}
}

func TestGetTagByName(t *testing.T) {
	s := tempDB(t)
	s.CreateTag(context.Background(), &models.Tag{Name: "mytag", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})

	got, err := s.GetTagByName(context.Background(), "mytag")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.Name != "mytag" {
		t.Errorf("expected name 'mytag', got %s", got.Name)
	}
}

func TestListTags(t *testing.T) {
	s := tempDB(t)
	s.CreateTag(context.Background(), &models.Tag{Name: "tag1", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	s.CreateTag(context.Background(), &models.Tag{Name: "tag2", StartCron: "0 9 * * *", StopCron: "0 19 * * *", Enabled: true})

	tags, err := s.ListTags(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}

func TestUpdateTag(t *testing.T) {
	s := tempDB(t)
	tag := &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateTag(context.Background(), tag)

	created.StartCron = "0 9 * * *"
	updated, err := s.UpdateTag(context.Background(), created)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.StartCron != "0 9 * * *" {
		t.Errorf("expected updated StartCron, got %s", updated.StartCron)
	}
}

func TestDeleteTag(t *testing.T) {
	s := tempDB(t)
	tag := &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateTag(context.Background(), tag)

	err := s.DeleteTag(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err = s.GetTag(context.Background(), created.ID)
	if err == nil {
		t.Error("expected error getting deleted tag")
	}
}

func TestDeleteTagCascadesSchedules(t *testing.T) {
	s := tempDB(t)
	tag, _ := s.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})

	tagID := tag.ID
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "other",
		DisplayName:   "other",
		StartCron:     "0 9 * * *",
		StopCron:      "0 19 * * *",
		Enabled:       true,
	})

	err := s.DeleteTag(context.Background(), tag.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	schedules, _ := s.ListSchedules(context.Background())
	if len(schedules) != 1 {
		t.Errorf("expected 1 schedule remaining (direct), got %d", len(schedules))
	}
	if schedules[0].ContainerName != "other" {
		t.Errorf("expected direct schedule 'other' to remain, got %s", schedules[0].ContainerName)
	}
}

func TestListSchedulesByTag(t *testing.T) {
	s := tempDB(t)
	tag, _ := s.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	tagID := tag.ID
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "app1",
		DisplayName:   "app1",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "app2",
		DisplayName:   "app2",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "standalone",
		DisplayName:   "standalone",
		StartCron:     "0 9 * * *",
		StopCron:      "0 19 * * *",
		Enabled:       true,
	})

	tagSchedules, err := s.ListSchedulesByTag(context.Background(), tagID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tagSchedules) != 2 {
		t.Errorf("expected 2 tag schedules, got %d", len(tagSchedules))
	}
}

func TestGetScheduleByTagAndContainer(t *testing.T) {
	s := tempDB(t)
	tag, _ := s.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	tagID := tag.ID
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})

	sched, err := s.GetScheduleByTagAndContainer(context.Background(), tagID, "my-app")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sched.ContainerName != "my-app" {
		t.Errorf("expected container_name 'my-app', got %s", sched.ContainerName)
	}
}

func TestDuplicateTagAndContainer(t *testing.T) {
	s := tempDB(t)
	tag, _ := s.CreateTag(context.Background(), &models.Tag{Name: "test", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	tagID := tag.ID
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})

	_, err := s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
		TagID:         &tagID,
	})
	if err == nil {
		t.Error("expected error for duplicate tag_id + container_name")
	}
}

func TestGetOnDemandSchedule(t *testing.T) {
	s := tempDB(t)

	_, err := s.GetOnDemandSchedule(context.Background(), "my-app")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}

	sched := &models.Schedule{
		ContainerName:   "my-app",
		DisplayName:     "My App",
		StackName:       "webstack",
		StartCron:       "0 8 * * 1-5",
		StopCron:        "0 18 * * 1-5",
		Enabled:         true,
		OnDemandEnabled: true,
		OnDemandURL:     "http://example.com",
		IdleTimeoutSec:  300,
	}
	created, _ := s.CreateSchedule(context.Background(), sched)

	got, err := s.GetOnDemandSchedule(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, got.ID)
	}
	if got.OnDemandEnabled != true {
		t.Error("expected OnDemandEnabled=true")
	}
	if got.OnDemandURL != "http://example.com" {
		t.Errorf("expected OnDemandURL 'http://example.com', got %s", got.OnDemandURL)
	}
	if got.IdleTimeoutSec != 300 {
		t.Errorf("expected IdleTimeoutSec 300, got %d", got.IdleTimeoutSec)
	}

	offSched := &models.Schedule{
		ContainerName:   "other-app",
		DisplayName:     "Other App",
		StartCron:       "0 9 * * *",
		StopCron:        "0 19 * * *",
		Enabled:         true,
		OnDemandEnabled: false,
	}
	s.CreateSchedule(context.Background(), offSched)

	_, err = s.GetOnDemandSchedule(context.Background(), "other-app")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for non-on-demand container, got %v", err)
	}
}

func TestUniqueTagContainerAllowsNull(t *testing.T) {
	s := tempDB(t)
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 8 * * *",
		StopCron:      "0 18 * * *",
		Enabled:       true,
	})
	s.CreateSchedule(context.Background(), &models.Schedule{
		ContainerName: "my-app",
		DisplayName:   "my-app",
		StartCron:     "0 9 * * *",
		StopCron:      "0 19 * * *",
		Enabled:       true,
	})

	schedules, _ := s.ListSchedules(context.Background())
	if len(schedules) != 2 {
		t.Errorf("expected 2 schedules with null tag_id and same container, got %d", len(schedules))
	}
}

// --- Stack tests ---

func TestCreateAndGetStack(t *testing.T) {
	s := tempDB(t)
	stack := &models.Stack{
		Name:             "myproject",
		DisplayName:      "My Project",
		StartCron:        "0 8 * * 1-5",
		StopCron:         "0 18 * * 1-5",
		Enabled:          true,
		OnDemandEnabled:  false,
		PrimaryContainer: "",
	}
	created, err := s.CreateStack(context.Background(), stack)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}

	got, err := s.GetStack(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.Name != "myproject" {
		t.Errorf("expected myproject, got %s", got.Name)
	}
}

func TestGetStackByName(t *testing.T) {
	s := tempDB(t)
	stack := &models.Stack{Name: "proj", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateStack(context.Background(), stack)

	got, err := s.GetStackByName(context.Background(), "proj")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, got.ID)
	}
}

func TestListStacks(t *testing.T) {
	s := tempDB(t)
	s.CreateStack(context.Background(), &models.Stack{Name: "a", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	s.CreateStack(context.Background(), &models.Stack{Name: "b", StartCron: "0 9 * * *", StopCron: "0 19 * * *", Enabled: true})

	stacks, err := s.ListStacks(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(stacks) != 2 {
		t.Errorf("expected 2, got %d", len(stacks))
	}
}

func TestUpdateStack(t *testing.T) {
	s := tempDB(t)
	stack := &models.Stack{Name: "proj", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateStack(context.Background(), stack)

	created.DisplayName = "Updated"
	created.Enabled = false
	updated, err := s.UpdateStack(context.Background(), created)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.DisplayName != "Updated" {
		t.Errorf("expected Updated, got %s", updated.DisplayName)
	}
	if updated.Enabled {
		t.Error("expected Enabled=false")
	}
}

func TestDeleteStack(t *testing.T) {
	s := tempDB(t)
	stack := &models.Stack{Name: "proj", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateStack(context.Background(), stack)

	if err := s.DeleteStack(context.Background(), created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_, err := s.GetStack(context.Background(), created.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestToggleStack(t *testing.T) {
	s := tempDB(t)
	stack := &models.Stack{Name: "proj", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true}
	created, _ := s.CreateStack(context.Background(), stack)

	toggled, err := s.ToggleStack(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if toggled.Enabled {
		t.Error("expected Enabled=false after toggle")
	}
}

func TestStackNameUnique(t *testing.T) {
	s := tempDB(t)
	s.CreateStack(context.Background(), &models.Stack{Name: "proj", StartCron: "0 8 * * *", StopCron: "0 18 * * *", Enabled: true})
	_, err := s.CreateStack(context.Background(), &models.Stack{Name: "proj", StartCron: "0 9 * * *", StopCron: "0 19 * * *", Enabled: true})
	if err == nil {
		t.Error("expected unique constraint error")
	}
}

func TestMigrationV5_TablesExist(t *testing.T) {
	s := tempDB(t)
	var version int
	if err := s.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("schema_version: %v", err)
	}
	if version < 5 {
		t.Fatalf("expected schema version >= 5, got %d", version)
	}
	if _, err := s.db.Exec("SELECT id, username, password_hash, role, oidc_subject, created_at, updated_at FROM users LIMIT 0"); err != nil {
		t.Fatalf("users table: %v", err)
	}
	if _, err := s.db.Exec("SELECT token, user_id, expires_at, created_at FROM sessions LIMIT 0"); err != nil {
		t.Fatalf("sessions table: %v", err)
	}
}