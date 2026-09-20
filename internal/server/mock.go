package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"mockflow/internal/matcher"
	"mockflow/internal/store"
)

type MockResult struct {
	StatusCode     int
	Headers        map[string]string
	Body           string
	Stage          *store.Stage
	Elapsed        int
	ResourceKey    string
	EndpointID     int64
	Matched        bool
	NotFoundReason string
}

func SelectStage(stages []store.Stage, elapsed int) *store.Stage {
	if len(stages) == 0 {
		return nil
	}
	sorted := append([]store.Stage(nil), stages...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].AfterSeconds == sorted[j].AfterSeconds {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].AfterSeconds < sorted[j].AfterSeconds
	})
	best := -1
	for i := range sorted {
		if sorted[i].AfterSeconds <= elapsed {
			best = i
		}
	}
	if best < 0 {
		return &sorted[0]
	}
	return &sorted[best]
}

func Substitute(s string, params map[string]string) string {
	if s == "" || len(params) == 0 {
		return s
	}
	out := s
	for k, v := range params {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

func (s *Server) ServeMock(projectID int64, method, path string, now time.Time) (MockResult, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = matcher.Normalize(path)

	var endpoints []store.Endpoint
	var err error
	if projectID > 0 {
		endpoints, err = s.store.ListAllByProject(projectID)
	} else {
		endpoints, err = s.store.ListAll()
	}
	if err != nil {
		return MockResult{}, err
	}

	var best *store.Endpoint
	var bestMatch matcher.Result
	for i := range endpoints {
		ep := &endpoints[i]
		if strings.ToUpper(ep.Method) != method {
			continue
		}
		m, ok := matcher.Match(ep.Path, path)
		if !ok {
			continue
		}
		if best == nil || m.Score > bestMatch.Score || (m.Score == bestMatch.Score && ep.ID < best.ID) {
			best = ep
			bestMatch = m
		}
	}
	if best == nil {
		return MockResult{Matched: false, NotFoundReason: "no matching mock endpoint"}, nil
	}

	key := bestMatch.ResourceKey
	var runtime store.RuntimeState
	switch method {
	case http.MethodPost:
		runtime, err = s.store.ResetRuntime(best.ID, key, now)
		if err != nil {
			return MockResult{}, err
		}
	case http.MethodDelete:
		existing, err := s.store.GetRuntime(best.ID, key)
		if err != nil {
			return MockResult{}, err
		}
		if existing != nil {
			runtime = *existing
		} else {
			runtime = store.RuntimeState{EndpointID: best.ID, ResourceKey: key, CreatedAt: now}
		}
	default:
		runtime, err = s.store.GetOrCreateRuntime(best.ID, key, now)
		if err != nil {
			return MockResult{}, err
		}
	}

	elapsed := int(now.Sub(runtime.CreatedAt) / time.Second)
	if elapsed < 0 {
		elapsed = 0
	}
	stage := SelectStage(best.Stages, elapsed)
	if stage == nil {
		return MockResult{
			Matched:        true,
			EndpointID:     best.ID,
			ResourceKey:    key,
			Elapsed:        elapsed,
			NotFoundReason: "endpoint has no response stages",
		}, nil
	}

	body := Substitute(stage.Body, bestMatch.Params)
	headers := map[string]string{}
	rawHeaders := Substitute(stage.Headers, bestMatch.Params)
	if strings.TrimSpace(rawHeaders) != "" {
		if err := json.Unmarshal([]byte(rawHeaders), &headers); err != nil {
			var obj map[string]any
			if err2 := json.Unmarshal([]byte(rawHeaders), &obj); err2 == nil {
				for k, v := range obj {
					headers[k] = fmt.Sprint(v)
				}
			} else {
				log.Printf("invalid headers JSON on stage %d: %v", stage.ID, err)
			}
		}
	}
	if _, ok := headerCI(headers, "Content-Type"); !ok {
		headers["Content-Type"] = "application/json"
	}

	if method == http.MethodDelete {
		if err := s.store.DeleteRuntime(best.ID, key); err != nil {
			return MockResult{}, err
		}
	}

	stCopy := *stage
	return MockResult{
		StatusCode:  stage.StatusCode,
		Headers:     headers,
		Body:        body,
		Stage:       &stCopy,
		Elapsed:     elapsed,
		ResourceKey: key,
		EndpointID:  best.ID,
		Matched:     true,
	}, nil
}

func headerCI(h map[string]string, key string) (string, bool) {
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

func setMockCORS(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "*")
	h.Set("Access-Control-Max-Age", "86400")
}

func (s *Server) mockHandler(projectID int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serveMockHTTP(w, r, projectID)
	})
}

func (s *Server) handleMock(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetProjectByPort(s.controlPort)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var pid int64
	if p != nil {
		pid = p.ID
	}
	s.serveMockHTTP(w, r, pid)
}

func (s *Server) serveMockHTTP(w http.ResponseWriter, r *http.Request, projectID int64) {
	setMockCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if projectID == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no project bound to this port"})
		return
	}

	now := s.now()
	res, err := s.ServeMock(projectID, r.Method, r.URL.Path, now)
	if err != nil {
		log.Printf("mock error %s %s: %v", r.Method, r.URL.Path, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !res.Matched || res.Stage == nil {
		reason := res.NotFoundReason
		if reason == "" {
			reason = "not found"
		}
		log.Printf("%s %s unmatched: %s", r.Method, r.URL.Path, reason)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": reason})
		return
	}
	log.Printf("%s %s stage=%d after=%ds elapsed=%ds status=%d",
		r.Method, r.URL.Path, res.Stage.ID, res.Stage.AfterSeconds, res.Elapsed, res.StatusCode)
	for k, v := range res.Headers {
		w.Header().Set(k, v)
	}
	setMockCORS(w)
	w.WriteHeader(res.StatusCode)
	_, _ = io.WriteString(w, res.Body)
}
