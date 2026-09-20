package matcher

import "testing"

func TestMatchParam(t *testing.T) {
	r, ok := Match("/orders/{id}", "/orders/1")
	if !ok {
		t.Fatal("expected match")
	}
	if r.Params["id"] != "1" {
		t.Fatalf("id=%q", r.Params["id"])
	}
	if r.ResourceKey != "1" {
		t.Fatalf("key=%q", r.ResourceKey)
	}
}

func TestMatchWildcard(t *testing.T) {
	r, ok := Match("/api/*", "/api/foo/bar")
	if !ok {
		t.Fatal("expected match")
	}
	if r.Params["*"] != "foo/bar" {
		t.Fatalf("*=%q", r.Params["*"])
	}
	if r.ResourceKey != "_" {
		t.Fatalf("key=%q", r.ResourceKey)
	}
}

func TestStaticBeatsParam(t *testing.T) {
	a, okA := Match("/orders/1", "/orders/1")
	b, okB := Match("/orders/{id}", "/orders/1")
	if !okA || !okB {
		t.Fatal("both should match")
	}
	if a.Score <= b.Score {
		t.Fatalf("static score %d should beat param %d", a.Score, b.Score)
	}
}
