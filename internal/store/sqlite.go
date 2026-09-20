package store

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS projects (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  port INTEGER NOT NULL UNIQUE,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS endpoints (
  id INTEGER PRIMARY KEY,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  description TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS response_stages (
  id INTEGER PRIMARY KEY,
  endpoint_id INTEGER NOT NULL,
  after_seconds INTEGER NOT NULL,
  status_code INTEGER DEFAULT 200,
  headers TEXT,
  body TEXT,
  FOREIGN KEY (endpoint_id) REFERENCES endpoints(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS runtime_state (
  id INTEGER PRIMARY KEY,
  endpoint_id INTEGER NOT NULL,
  resource_key TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(endpoint_id, resource_key)
);

CREATE INDEX IF NOT EXISTS idx_stages_endpoint ON response_stages(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_runtime_endpoint_key ON runtime_state(endpoint_id, resource_key);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := s.ensureDefaultProject(); err != nil {
		return err
	}
	if err := s.ensureEndpointProjectColumn(); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureDefaultProject() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		return fmt.Errorf("count projects: %w", err)
	}
	if n > 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO projects (name, description, port) VALUES (?, ?, ?)`,
		"Default", "默认项目", 8080)
	if err != nil {
		return fmt.Errorf("insert default project: %w", err)
	}
	return nil
}

func (s *Store) ensureEndpointProjectColumn() error {
	rows, err := s.db.Query(`PRAGMA table_info(endpoints)`)
	if err != nil {
		return fmt.Errorf("pragma table_info: %w", err)
	}
	defer rows.Close()
	has := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "project_id" {
			has = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if has {
		return nil
	}
	var pid int64
	if err := s.db.QueryRow(`SELECT id FROM projects ORDER BY id LIMIT 1`).Scan(&pid); err != nil {
		return fmt.Errorf("default project id: %w", err)
	}
	if _, err := s.db.Exec(fmt.Sprintf(`ALTER TABLE endpoints ADD COLUMN project_id INTEGER NOT NULL DEFAULT %d`, pid)); err != nil {
		return fmt.Errorf("add project_id: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_endpoints_project ON endpoints(project_id)`); err != nil {
		return fmt.Errorf("index project_id: %w", err)
	}
	return nil
}

func parseTime(v string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
		return t
	}
	return time.Time{}
}

func (s *Store) CountEndpoints() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM endpoints`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count endpoints: %w", err)
	}
	return n, nil
}

func (s *Store) ListEndpoints(projectID int64) ([]Endpoint, error) {
	q := `
SELECT e.id, e.project_id, e.method, e.path, COALESCE(e.description, ''), e.created_at,
       (SELECT COUNT(*) FROM response_stages s WHERE s.endpoint_id = e.id)
FROM endpoints e`
	args := []any{}
	if projectID > 0 {
		q += ` WHERE e.project_id = ?`
		args = append(args, projectID)
	}
	q += ` ORDER BY e.id`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list endpoints: %w", err)
	}
	defer rows.Close()
	var out []Endpoint
	for rows.Next() {
		var e Endpoint
		var created string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Method, &e.Path, &e.Description, &created, &e.StageCount); err != nil {
			return nil, fmt.Errorf("scan endpoint: %w", err)
		}
		e.CreatedAt = parseTime(created)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Endpoint{}
	}
	return out, nil
}

func (s *Store) ListAll() ([]Endpoint, error) {
	return s.ListAllByProject(0)
}

func (s *Store) ListAllByProject(projectID int64) ([]Endpoint, error) {
	eps, err := s.ListEndpoints(projectID)
	if err != nil {
		return nil, err
	}
	for i := range eps {
		stages, err := s.listStages(eps[i].ID)
		if err != nil {
			return nil, err
		}
		eps[i].Stages = stages
	}
	return eps, nil
}

func (s *Store) GetEndpoint(id int64) (*Endpoint, error) {
	var e Endpoint
	var created string
	err := s.db.QueryRow(`
SELECT id, project_id, method, path, COALESCE(description, ''), created_at
FROM endpoints WHERE id = ?`, id).Scan(&e.ID, &e.ProjectID, &e.Method, &e.Path, &e.Description, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get endpoint: %w", err)
	}
	e.CreatedAt = parseTime(created)
	stages, err := s.listStages(id)
	if err != nil {
		return nil, err
	}
	e.Stages = stages
	e.StageCount = len(stages)
	return &e, nil
}

func (s *Store) listStages(endpointID int64) ([]Stage, error) {
	rows, err := s.db.Query(`
SELECT id, endpoint_id, after_seconds, status_code, COALESCE(headers, ''), COALESCE(body, '')
FROM response_stages WHERE endpoint_id = ? ORDER BY after_seconds, id`, endpointID)
	if err != nil {
		return nil, fmt.Errorf("list stages: %w", err)
	}
	defer rows.Close()
	var out []Stage
	for rows.Next() {
		var st Stage
		if err := rows.Scan(&st.ID, &st.EndpointID, &st.AfterSeconds, &st.StatusCode, &st.Headers, &st.Body); err != nil {
			return nil, fmt.Errorf("scan stage: %w", err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Stage{}
	}
	return out, nil
}

func (s *Store) CreateEndpoint(e Endpoint) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if e.ProjectID == 0 {
		pid, err := s.DefaultProjectID()
		if err != nil {
			return 0, err
		}
		e.ProjectID = pid
	}
	res, err := tx.Exec(`INSERT INTO endpoints (project_id, method, path, description) VALUES (?, ?, ?, ?)`,
		e.ProjectID, e.Method, e.Path, e.Description)
	if err != nil {
		return 0, fmt.Errorf("insert endpoint: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := insertStages(tx, id, e.Stages); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateEndpoint(e Endpoint) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`UPDATE endpoints SET project_id = ?, method = ?, path = ?, description = ? WHERE id = ?`,
		e.ProjectID, e.Method, e.Path, e.Description, e.ID)
	if err != nil {
		return fmt.Errorf("update endpoint: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("endpoint %d not found", e.ID)
	}
	if _, err := tx.Exec(`DELETE FROM response_stages WHERE endpoint_id = ?`, e.ID); err != nil {
		return fmt.Errorf("delete stages: %w", err)
	}
	if err := insertStages(tx, e.ID, e.Stages); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func insertStages(tx *sql.Tx, endpointID int64, stages []Stage) error {
	for _, st := range stages {
		status := st.StatusCode
		if status == 0 {
			status = 200
		}
		if _, err := tx.Exec(`
INSERT INTO response_stages (endpoint_id, after_seconds, status_code, headers, body)
VALUES (?, ?, ?, ?, ?)`, endpointID, st.AfterSeconds, status, st.Headers, st.Body); err != nil {
			return fmt.Errorf("insert stage: %w", err)
		}
	}
	return nil
}

func (s *Store) DeleteEndpoint(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM runtime_state WHERE endpoint_id = ?`, id); err != nil {
		return fmt.Errorf("delete runtime: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM endpoints WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete endpoint: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("endpoint %d not found", id)
	}
	return nil
}

func (s *Store) DuplicateEndpoint(id int64) (int64, error) {
	e, err := s.GetEndpoint(id)
	if err != nil {
		return 0, err
	}
	if e == nil {
		return 0, fmt.Errorf("endpoint %d not found", id)
	}
	desc := strings.TrimSpace(e.Description)
	if desc == "" {
		desc = "copy"
	} else {
		desc = desc + " (copy)"
	}
	e.Description = desc
	e.ID = 0
	return s.CreateEndpoint(*e)
}

func (s *Store) GetOrCreateRuntime(endpointID int64, key string, now time.Time) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return RuntimeState{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	st, ok, err := getRuntime(tx, endpointID, key)
	if err != nil {
		return RuntimeState{}, err
	}
	if !ok {
		if _, err := tx.Exec(`
INSERT INTO runtime_state (endpoint_id, resource_key, created_at) VALUES (?, ?, ?)`,
			endpointID, key, now.UTC().Format(time.RFC3339Nano)); err != nil {
			return RuntimeState{}, fmt.Errorf("insert runtime: %w", err)
		}
		st, ok, err = getRuntime(tx, endpointID, key)
		if err != nil {
			return RuntimeState{}, err
		}
		if !ok {
			return RuntimeState{}, fmt.Errorf("runtime state missing after insert")
		}
	}
	if err := tx.Commit(); err != nil {
		return RuntimeState{}, fmt.Errorf("commit: %w", err)
	}
	return st, nil
}

func (s *Store) ResetRuntime(endpointID int64, key string, now time.Time) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return RuntimeState{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM runtime_state WHERE endpoint_id = ? AND resource_key = ?`, endpointID, key); err != nil {
		return RuntimeState{}, fmt.Errorf("delete runtime: %w", err)
	}
	if _, err := tx.Exec(`
INSERT INTO runtime_state (endpoint_id, resource_key, created_at) VALUES (?, ?, ?)`,
		endpointID, key, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return RuntimeState{}, fmt.Errorf("insert runtime: %w", err)
	}
	st, ok, err := getRuntime(tx, endpointID, key)
	if err != nil {
		return RuntimeState{}, err
	}
	if !ok {
		return RuntimeState{}, fmt.Errorf("runtime state missing after reset")
	}
	if err := tx.Commit(); err != nil {
		return RuntimeState{}, fmt.Errorf("commit: %w", err)
	}
	return st, nil
}

func (s *Store) DeleteRuntime(endpointID int64, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM runtime_state WHERE endpoint_id = ? AND resource_key = ?`, endpointID, key); err != nil {
		return fmt.Errorf("delete runtime: %w", err)
	}
	return nil
}

func (s *Store) GetRuntime(endpointID int64, key string) (*RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var st RuntimeState
	var created string
	err := s.db.QueryRow(`
SELECT id, endpoint_id, resource_key, created_at FROM runtime_state
WHERE endpoint_id = ? AND resource_key = ?`, endpointID, key).Scan(&st.ID, &st.EndpointID, &st.ResourceKey, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get runtime: %w", err)
	}
	st.CreatedAt = parseTime(created)
	return &st, nil
}

func getRuntime(tx *sql.Tx, endpointID int64, key string) (RuntimeState, bool, error) {
	var st RuntimeState
	var created string
	err := tx.QueryRow(`
SELECT id, endpoint_id, resource_key, created_at FROM runtime_state
WHERE endpoint_id = ? AND resource_key = ?`, endpointID, key).Scan(&st.ID, &st.EndpointID, &st.ResourceKey, &created)
	if err == sql.ErrNoRows {
		return RuntimeState{}, false, nil
	}
	if err != nil {
		return RuntimeState{}, false, fmt.Errorf("get runtime: %w", err)
	}
	st.CreatedAt = parseTime(created)
	return st, true, nil
}

func (s *Store) ReplaceAll(projects []Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM runtime_state`); err != nil {
		return fmt.Errorf("clear runtime: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM response_stages`); err != nil {
		return fmt.Errorf("clear stages: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM endpoints`); err != nil {
		return fmt.Errorf("clear endpoints: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM projects`); err != nil {
		return fmt.Errorf("clear projects: %w", err)
	}
	for _, p := range projects {
		if p.Port == 0 {
			p.Port = 8080
		}
		if p.Name == "" {
			p.Name = "Default"
		}
		res, err := tx.Exec(`INSERT INTO projects (name, description, port) VALUES (?, ?, ?)`,
			p.Name, p.Description, p.Port)
		if err != nil {
			return fmt.Errorf("insert project: %w", err)
		}
		pid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, e := range p.Endpoints {
			er, err := tx.Exec(`INSERT INTO endpoints (project_id, method, path, description) VALUES (?, ?, ?, ?)`,
				pid, e.Method, e.Path, e.Description)
			if err != nil {
				return fmt.Errorf("insert endpoint: %w", err)
			}
			id, err := er.LastInsertId()
			if err != nil {
				return err
			}
			if err := insertStages(tx, id, e.Stages); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
