package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sqlx.DB
}

type User struct {
	ID             int64  `db:"id"`
	TGID           int64  `db:"tg_id"`
	TZ             string `db:"tz"`
	ControlEnabled bool   `db:"control_enabled"`
}

type Task struct {
	ID       int64  `db:"id"`
	UserID   int64  `db:"user_id"`
	Title    string `db:"title"`
	StartH   int    `db:"start_h"`
	StartM   int    `db:"start_m"`
	EndH     int    `db:"end_h"`
	EndM     int    `db:"end_m"`
	DaysMask int    `db:"days_mask"`
	Enabled  bool   `db:"enabled"`
}

type TaskRun struct {
	ID      int64  `db:"id"`
	UserID  int64  `db:"user_id"`
	TaskID  int64  `db:"task_id"`
	StartTs int64  `db:"start_ts"`
	EndTs   *int64 `db:"end_ts"`
}

type StatRow struct {
	Title   string
	Seconds int64
}

func Open(databaseURL string) (*Store, error) {
	db, err := sqlx.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout=5000&_pragma=foreign_keys(1)", databaseURL))
	if err != nil { return nil, err }
	if err := db.Ping(); err != nil { return nil, err }
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil { return nil, err }
	if err := runMigrations(db); err != nil { return nil, err }
	return &Store{DB: db}, nil
}

func (s *Store) GetOrCreateUser(tgID int64, defaultTZ string) (User, error) {
	var u User
	err := s.DB.Get(&u, "SELECT id, tg_id, tz, control_enabled FROM users WHERE tg_id = ?", tgID)
	if err == nil { return u, nil }
	if !errors.Is(err, sql.ErrNoRows) { return u, err }
	res, err := s.DB.Exec("INSERT INTO users (tg_id, tz, control_enabled) VALUES (?, ?, 0)", tgID, defaultTZ)
	if err != nil { return u, err }
	id, _ := res.LastInsertId()
	u = User{ID: id, TGID: tgID, TZ: defaultTZ, ControlEnabled: false}
	return u, nil
}

func (s *Store) GetUserByTGID(tgID int64) (User, error) {
	var u User
	err := s.DB.Get(&u, "SELECT id, tg_id, tz, control_enabled FROM users WHERE tg_id = ?", tgID)
	return u, err
}

func (s *Store) UpdateUserTZ(userID int64, tz string) error {
	_, err := s.DB.Exec("UPDATE users SET tz = ? WHERE id = ?", tz, userID)
	return err
}

func (s *Store) SetControl(userID int64, enabled bool) error {
	val := 0
	if enabled { val = 1 }
	_, err := s.DB.Exec("UPDATE users SET control_enabled = ? WHERE id = ?", val, userID)
	return err
}

func (s *Store) CreateTask(t Task) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO tasks (user_id, title, start_h, start_m, end_h, end_m, days_mask, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)`, t.UserID, t.Title, t.StartH, t.StartM, t.EndH, t.EndM, t.DaysMask)
	if err != nil { return 0, err }
	return res.LastInsertId()
}

func (s *Store) ListTasks(userID int64) ([]Task, error) {
	var tasks []Task
	err := s.DB.Select(&tasks, `SELECT id, user_id, title, start_h, start_m, end_h, end_m, days_mask, enabled
		FROM tasks WHERE user_id = ? ORDER BY start_h, start_m`, userID)
	return tasks, err
}

func (s *Store) GetTask(userID, taskID int64) (Task, error) {
	var t Task
	err := s.DB.Get(&t, `SELECT id, user_id, title, start_h, start_m, end_h, end_m, days_mask, enabled
		FROM tasks WHERE id = ? AND user_id = ?`, taskID, userID)
	return t, err
}

func (s *Store) DeleteTask(userID, taskID int64) error {
	_, err := s.DB.Exec("DELETE FROM tasks WHERE id = ? AND user_id = ?", taskID, userID)
	return err
}

func (s *Store) ToggleTask(userID, taskID int64) (bool, error) {
	var cur int
	err := s.DB.Get(&cur, "SELECT enabled FROM tasks WHERE id = ? AND user_id = ?", taskID, userID)
	if err != nil { return false, err }
	newVal := 1
	if cur == 1 { newVal = 0 }
	_, err = s.DB.Exec("UPDATE tasks SET enabled = ? WHERE id = ? AND user_id = ?", newVal, taskID, userID)
	return newVal == 1, err
}

// --- Time tracking ---
func (s *Store) StartRun(userID, taskID int64, start time.Time) error {
	var openCount int
	if err := s.DB.Get(&openCount, "SELECT COUNT(1) FROM task_runs WHERE user_id=? AND task_id=? AND end_ts IS NULL", userID, taskID); err == nil && openCount > 0 {
		return nil
	}
	_, err := s.DB.Exec("INSERT INTO task_runs (user_id, task_id, start_ts) VALUES (?, ?, ?)", userID, taskID, start.Unix())
	return err
}

func (s *Store) EndRun(userID, taskID int64, end time.Time) error {
	_, err := s.DB.Exec(`UPDATE task_runs
		SET end_ts = ?
		WHERE id = (
			SELECT id FROM task_runs
			WHERE user_id = ? AND task_id = ? AND end_ts IS NULL
			ORDER BY start_ts DESC
			LIMIT 1
		)`, end.Unix(), userID, taskID)
	return err
}

func (s *Store) GetStats(userID int64, fromUTC, toUTC time.Time) ([]StatRow, error) {
	type row struct {
		Title string `db:"title"`
		Start int64  `db:"start_ts"`
		End   *int64 `db:"end_ts"`
	}
	var rows []row
	err := s.DB.Select(&rows, `
		SELECT t.title as title, r.start_ts, r.end_ts
		FROM task_runs r
		JOIN tasks t ON t.id = r.task_id
		WHERE r.user_id = ?
		  AND r.start_ts < ?
		  AND (r.end_ts IS NULL OR r.end_ts > ?)
	`, userID, toUTC.Unix(), fromUTC.Unix())
	if err != nil { return nil, err }

	now := time.Now().UTC().Unix()
	acc := map[string]int64{}
	from := fromUTC.Unix()
	to := toUTC.Unix()

	for _, r := range rows {
		start := r.Start
		end := to
		if r.End != nil { end = *r.End } else { end = now }
		if start < from { start = from }
		if end > to { end = to }
		dur := end - start
		if dur < 0 { dur = 0 }
		acc[r.Title] += dur
	}
	out := make([]StatRow, 0, len(acc))
	for title, sec := range acc {
		out = append(out, StatRow{Title: title, Seconds: sec})
	}
	return out, nil
}


func (s *Store) GetTasksForUser(userID int64) ([]Task, error) {
    var tasks []Task
    err := s.DB.Select(&tasks, `SELECT id, user_id, title, start_h, start_m, end_h, end_m, days_mask, enabled FROM tasks WHERE user_id = ? AND enabled = 1 ORDER BY start_h, start_m`, userID)
    return tasks, err
}

func (s *Store) UsersWithControlEnabled() ([]User, error) {
    var users []User
    err := s.DB.Select(&users, `SELECT id, tg_id, tz, control_enabled FROM users WHERE control_enabled = 1`)
    return users, err
}
