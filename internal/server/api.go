package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"mockflow/internal/config"
	"mockflow/internal/matcher"
	"mockflow/internal/store"
)

type Server struct {
	store       *store.Store
	web         fs.FS
	now         func() time.Time
	mux         *http.ServeMux
	dbPath      string
	controlPort int
	hub         *Hub
}

func New(st *store.Store, webFS fs.FS, dbPath string, controlPort int) *Server {
	if controlPort == 0 {
		controlPort = 8080
	}
	s := &Server{
		store:       st,
		web:         webFS,
		now:         func() time.Time { return time.Now() },
		dbPath:      dbPath,
		controlPort: controlPort,
	}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

func (s *Server) SetNow(fn func() time.Time) {
	s.now = fn
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /_api/meta", s.handleMeta)
	s.mux.HandleFunc("GET /_api/projects", s.handleListProjects)
	s.mux.HandleFunc("POST /_api/projects", s.handleCreateProject)
	s.mux.HandleFunc("GET /_api/projects/{id}", s.handleGetProject)
	s.mux.HandleFunc("PUT /_api/projects/{id}", s.handleUpdateProject)
	s.mux.HandleFunc("DELETE /_api/projects/{id}", s.handleDeleteProject)
	s.mux.HandleFunc("GET /_api/endpoints", s.handleListEndpoints)
	s.mux.HandleFunc("POST /_api/endpoints", s.handleCreateEndpoint)
	s.mux.HandleFunc("GET /_api/endpoints/{id}", s.handleGetEndpoint)
	s.mux.HandleFunc("PUT /_api/endpoints/{id}", s.handleUpdateEndpoint)
	s.mux.HandleFunc("DELETE /_api/endpoints/{id}", s.handleDeleteEndpoint)
	s.mux.HandleFunc("POST /_api/endpoints/{id}/duplicate", s.handleDuplicateEndpoint)
	s.mux.HandleFunc("POST /_api/invoke", s.handleInvoke)
	s.mux.HandleFunc("DELETE /_api/runtime", s.handleResetRuntime)
	s.mux.HandleFunc("GET /_api/export", s.handleExport)
	s.mux.HandleFunc("POST /_api/import", s.handleImport)

	s.mux.Handle("/_ui/", http.StripPrefix("/_ui/", s.uiHandler()))
	s.mux.HandleFunc("/_ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/_ui/", http.StatusFound)
	})
	s.mux.HandleFunc("/", s.handleMock)
}

func (s *Server) uiHandler() http.Handler {
	if s.web == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "UI not embedded", http.StatusNotFound)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		if _, err := fs.Stat(s.web, p); err != nil {
			if strings.HasPrefix(p, "assets/") {
				http.NotFound(w, r)
				return
			}
			p = "index.html"
		}
		http.ServeFileFS(w, r, s.web, p)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func validateEndpoint(e *store.Endpoint) error {
	e.Method = strings.ToUpper(strings.TrimSpace(e.Method))
	e.Path = strings.TrimSpace(e.Path)
	switch e.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		return fmt.Errorf("invalid method %q", e.Method)
	}
	if e.Path == "" {
		return fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(e.Path, "/") {
		e.Path = "/" + e.Path
	}
	norm := matcher.Normalize(e.Path)
	if strings.HasPrefix(norm, "/_ui") || strings.HasPrefix(norm, "/_api") {
		return fmt.Errorf("path %q is reserved", e.Path)
	}
	for i := range e.Stages {
		if e.Stages[i].AfterSeconds < 0 {
			return fmt.Errorf("stage %d: after_seconds must be >= 0", i)
		}
		if e.Stages[i].StatusCode == 0 {
			e.Stages[i].StatusCode = 200
		}
		if e.Stages[i].StatusCode < 100 || e.Stages[i].StatusCode > 599 {
			return fmt.Errorf("stage %d: invalid status_code", i)
		}
		if e.Stages[i].Headers == "" {
			e.Stages[i].Headers = "{}"
		}
	}
	return nil
}

func (s *Server) syncListeners() {
	if s.hub != nil {
		if err := s.hub.SyncExtra(); err != nil {
			log.Printf("sync listeners: %v", err)
		}
	}
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"control_port": s.controlPort})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListProjects()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	p, err := s.store.GetProject(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var p store.Project
	if err := readJSON(r, &p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	id, err := s.store.CreateProject(p)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.hubSyncErr(); err != nil {
		_ = s.store.DeleteProject(id)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := s.store.GetProject(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var p store.Project
	if err := readJSON(r, &p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	p.ID = id
	if err := s.store.UpdateProject(p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.hubSyncErr(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	updated, err := s.store.GetProject(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteProject(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.syncListeners()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) hubSyncErr() error {
	if s.hub == nil {
		return nil
	}
	return s.hub.SyncExtra()
}

func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	var projectID int64
	if q := r.URL.Query().Get("project_id"); q != "" {
		id, err := strconv.ParseInt(q, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid project_id"})
			return
		}
		projectID = id
	}
	list, err := s.store.ListEndpoints(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	e, err := s.store.GetEndpoint(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if e == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var e store.Endpoint
	if err := readJSON(r, &e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if e.ProjectID == 0 {
		pid, err := s.store.DefaultProjectID()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		e.ProjectID = pid
	}
	if err := validateEndpoint(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	id, err := s.store.CreateEndpoint(e)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	created, err := s.store.GetEndpoint(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var e store.Endpoint
	if err := readJSON(r, &e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	e.ID = id
	if e.ProjectID == 0 {
		existing, err := s.store.GetEndpoint(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if existing != nil {
			e.ProjectID = existing.ProjectID
		}
	}
	if err := validateEndpoint(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.UpdateEndpoint(e); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	updated, err := s.store.GetEndpoint(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteEndpoint(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDuplicateEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	newID, err := s.store.DuplicateEndpoint(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	created, err := s.store.GetEndpoint(newID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

type invokeRequest struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	ProjectID int64             `json:"project_id"`
}

func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	var req invokeRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	res, err := s.ServeMock(req.ProjectID, req.Method, req.Path, s.now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{
		"matched":         res.Matched,
		"status_code":     res.StatusCode,
		"headers":         res.Headers,
		"body":            res.Body,
		"elapsed_seconds": res.Elapsed,
		"resource_key":    res.ResourceKey,
		"endpoint_id":     res.EndpointID,
		"error":           res.NotFoundReason,
	}
	if res.Stage != nil {
		out["stage_id"] = res.Stage.ID
		out["after_seconds"] = res.Stage.AfterSeconds
		out["status_code"] = res.StatusCode
	}
	if !res.Matched || res.Stage == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleResetRuntime(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("endpoint_id")
	key := r.URL.Query().Get("resource_key")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint_id is required"})
		return
	}
	if key == "" {
		key = "_"
	}
	if err := s.store.DeleteRuntime(id, key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjectsWithEndpoints()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	data, err := config.Export(projects)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=mockflow.yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	var data []byte
	var err error
	if strings.Contains(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file field required"})
			return
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	} else {
		data, err = io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	projects, err := config.Import(data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n := 0
	for _, p := range projects {
		n += len(p.Endpoints)
	}
	if err := s.store.ReplaceAll(projects); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.hubSyncErr(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": n, "projects": len(projects)})
}
