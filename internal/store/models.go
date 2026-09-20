package store

import "time"

type Project struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Port          int        `json:"port"`
	CreatedAt     time.Time  `json:"created_at"`
	EndpointCount int        `json:"endpoint_count,omitempty"`
	Endpoints     []Endpoint `json:"endpoints,omitempty"`
}

type Endpoint struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	StageCount  int       `json:"stage_count,omitempty"`
	Stages      []Stage   `json:"stages,omitempty"`
}

type Stage struct {
	ID           int64  `json:"id"`
	EndpointID   int64  `json:"endpoint_id"`
	AfterSeconds int    `json:"after_seconds"`
	StatusCode   int    `json:"status_code"`
	Headers      string `json:"headers"`
	Body         string `json:"body"`
}

type RuntimeState struct {
	ID          int64     `json:"id"`
	EndpointID  int64     `json:"endpoint_id"`
	ResourceKey string    `json:"resource_key"`
	CreatedAt   time.Time `json:"created_at"`
}

func ValidPort(port int) bool {
	return port >= 1024 && port <= 65535
}
