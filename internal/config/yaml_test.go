package config

import (
	"strings"
	"testing"
)

func TestImportLineNumber(t *testing.T) {
	src := "version: 9\nendpoints: []\n"
	_, err := Import([]byte(src))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error should include line number: %v", err)
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	src := `version: 1
endpoints:
  - method: GET
    path: /orders/{id}
    description: 订单状态查询
    stages:
      - after_seconds: 0
        status_code: 200
        headers:
          Content-Type: application/json
        body:
          id: "{id}"
          status: processing
`
	projects, err := Import([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || len(projects[0].Endpoints) != 1 {
		t.Fatalf("unexpected: %+v", projects)
	}
	eps := projects[0].Endpoints
	if !strings.Contains(eps[0].Stages[0].Body, "processing") {
		t.Fatalf("body=%s", eps[0].Stages[0].Body)
	}
	out, err := Export(projects)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "version: 2") {
		t.Fatalf("expected v2 export: %s", out)
	}
	again, err := Import(out)
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Endpoints[0].Path != "/orders/{id}" {
		t.Fatalf("path=%s", again[0].Endpoints[0].Path)
	}
}
