package store

import (
	"database/sql"
	"fmt"
	"strings"
)

func (s *Store) DefaultProjectID() (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM projects ORDER BY id LIMIT 1`).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("default project: %w", err)
	}
	return id, nil
}

func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`
SELECT p.id, p.name, COALESCE(p.description, ''), p.port, p.created_at,
       (SELECT COUNT(*) FROM endpoints e WHERE e.project_id = p.id)
FROM projects p
ORDER BY p.id`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		var created string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Port, &created, &p.EndpointCount); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.CreatedAt = parseTime(created)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Project{}
	}
	return out, nil
}

func (s *Store) ListProjectsWithEndpoints() ([]Project, error) {
	ps, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	for i := range ps {
		eps, err := s.ListAllByProject(ps[i].ID)
		if err != nil {
			return nil, err
		}
		ps[i].Endpoints = eps
	}
	return ps, nil
}

func (s *Store) GetProject(id int64) (*Project, error) {
	var p Project
	var created string
	err := s.db.QueryRow(`
SELECT id, name, COALESCE(description, ''), port, created_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Description, &p.Port, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	p.CreatedAt = parseTime(created)
	return &p, nil
}

func (s *Store) GetProjectByPort(port int) (*Project, error) {
	var p Project
	var created string
	err := s.db.QueryRow(`
SELECT id, name, COALESCE(description, ''), port, created_at FROM projects WHERE port = ?`, port).
		Scan(&p.ID, &p.Name, &p.Description, &p.Port, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project by port: %w", err)
	}
	p.CreatedAt = parseTime(created)
	return &p, nil
}

func (s *Store) CreateProject(p Project) (int64, error) {
	if err := validateProject(&p); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO projects (name, description, port) VALUES (?, ?, ?)`,
		p.Name, p.Description, p.Port)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, fmt.Errorf("port %d is already used by another project", p.Port)
		}
		return 0, fmt.Errorf("insert project: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) UpdateProject(p Project) error {
	if err := validateProject(&p); err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE projects SET name = ?, description = ?, port = ? WHERE id = ?`,
		p.Name, p.Description, p.Port, p.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return fmt.Errorf("port %d is already used by another project", p.Port)
		}
		return fmt.Errorf("update project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("project %d not found", p.ID)
	}
	return nil
}

func (s *Store) DeleteProject(id int64) error {
	eps, err := s.ListEndpoints(id)
	if err != nil {
		return err
	}
	for _, e := range eps {
		if _, err := s.db.Exec(`DELETE FROM runtime_state WHERE endpoint_id = ?`, e.ID); err != nil {
			return fmt.Errorf("delete runtime: %w", err)
		}
	}
	if _, err := s.db.Exec(`DELETE FROM endpoints WHERE project_id = ?`, id); err != nil {
		return fmt.Errorf("delete endpoints: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("project %d not found", id)
	}
	return nil
}

func validateProject(p *Project) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if !ValidPort(p.Port) {
		return fmt.Errorf("port must be between 1024 and 65535")
	}
	return nil
}
