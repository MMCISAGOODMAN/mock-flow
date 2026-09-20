package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mockflow/internal/store"
)

func setupServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s := New(st, nil, dir, 8080)
	pid, err := st.DefaultProjectID()
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateEndpoint(store.Endpoint{
		ProjectID:   pid,
		Method:      "GET",
		Path:        "/orders/{id}",
		Description: "order status",
		Stages: []store.Stage{
			{AfterSeconds: 0, StatusCode: 200, Body: `{"id":"{id}","status":"processing"}`},
			{AfterSeconds: 5, StatusCode: 200, Body: `{"id":"{id}","status":"shipped"}`},
			{AfterSeconds: 30, StatusCode: 200, Body: `{"id":"{id}","status":"delivered"}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFirstRequestCreatesState(t *testing.T) {
	s := setupServer(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err := s.ServeMock(0, "GET", "/orders/1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Stage == nil {
		t.Fatalf("unmatched: %+v", res)
	}
	if res.Elapsed != 0 {
		t.Fatalf("elapsed=%d", res.Elapsed)
	}
	if res.Stage.AfterSeconds != 0 {
		t.Fatalf("stage after=%d", res.Stage.AfterSeconds)
	}
	if res.Body != `{"id":"1","status":"processing"}` {
		t.Fatalf("body=%s", res.Body)
	}
	rt, err := s.store.GetRuntime(res.EndpointID, "1")
	if err != nil {
		t.Fatal(err)
	}
	if rt == nil {
		t.Fatal("expected runtime state to be created")
	}
}

func TestSecondStageAfter5Seconds(t *testing.T) {
	s := setupServer(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.ServeMock(0, "GET", "/orders/1", t0); err != nil {
		t.Fatal(err)
	}
	res, err := s.ServeMock(0, "GET", "/orders/1", t0.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if res.Elapsed != 5 {
		t.Fatalf("elapsed=%d", res.Elapsed)
	}
	if res.Stage == nil || res.Stage.AfterSeconds != 5 {
		t.Fatalf("expected 5s stage, got %+v", res.Stage)
	}
	if res.Body != `{"id":"1","status":"shipped"}` {
		t.Fatalf("body=%s", res.Body)
	}
}

func TestLastStageWhenElapsedExceedsAll(t *testing.T) {
	s := setupServer(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.ServeMock(0, "GET", "/orders/1", t0); err != nil {
		t.Fatal(err)
	}
	res, err := s.ServeMock(0, "GET", "/orders/1", t0.Add(120*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if res.Elapsed != 120 {
		t.Fatalf("elapsed=%d", res.Elapsed)
	}
	if res.Stage == nil || res.Stage.AfterSeconds != 30 {
		t.Fatalf("expected last stage, got %+v", res.Stage)
	}
	if res.Body != `{"id":"1","status":"delivered"}` {
		t.Fatalf("body=%s", res.Body)
	}
}

func TestMockCORSPreflightAndResponse(t *testing.T) {
	s := setupServer(t)

	pre := httptest.NewRequest(http.MethodOptions, "/orders/1", nil)
	pre.Header.Set("Origin", "http://localhost:5173")
	pre.Header.Set("Access-Control-Request-Method", "GET")
	preRec := httptest.NewRecorder()
	s.handleMock(preRec, pre)
	if preRec.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d", preRec.Code)
	}
	if got := preRec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("preflight ACAO=%q", got)
	}

	get := httptest.NewRequest(http.MethodGet, "/orders/1", nil)
	getRec := httptest.NewRecorder()
	s.handleMock(getRec, get)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	if got := getRec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("get ACAO=%q", got)
	}
}

func TestServeMockFiltersByProject(t *testing.T) {
	s := setupServer(t)
	b, err := s.store.CreateProject(store.Project{Name: "B", Port: 8081})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.store.CreateEndpoint(store.Endpoint{
		ProjectID: b,
		Method:    "GET",
		Path:      "/orders/{id}",
		Stages:    []store.Stage{{AfterSeconds: 0, StatusCode: 200, Body: `{"status":"from-b"}`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := s.store.DefaultProjectID()
	if err != nil {
		t.Fatal(err)
	}
	ra, err := s.ServeMock(a, "GET", "/orders/1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ra.Body, "processing") {
		t.Fatalf("project A body=%s", ra.Body)
	}
	rb, err := s.ServeMock(b, "GET", "/orders/1", now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Body != `{"status":"from-b"}` {
		t.Fatalf("project B body=%s", rb.Body)
	}
}
