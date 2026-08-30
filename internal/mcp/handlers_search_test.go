package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestHandleSearchObject_ServerSideTypeFilter verifies the MCP search path goes
// through SearchObjectByType: the short-form type is canonicalized and sent to
// the server as the objectType query param, and maxResults is forwarded — so
// max applies after the type filter (the bug txape10 flagged on PR #126).
func TestHandleSearchObject_ServerSideTypeFilter(t *testing.T) {
	var searchQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "informationsystem/search") {
			searchQuery = r.URL.RawQuery
		}
		w.Header().Set("X-CSRF-Token", "test-token")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"/>`))
	}))
	defer ts.Close()

	cfg := &Config{
		BaseURL:            ts.URL,
		Username:           "u",
		Password:           "p",
		Client:             "001",
		Language:           "EN",
		InsecureSkipVerify: true,
	}
	server := NewServer(cfg)
	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	req := newRequest(map[string]any{
		"query":      "Z*",
		"objectType": "CLAS",
		"maxResults": float64(5),
	})
	res, err := server.handleSearchObject(context.Background(), req)
	if err != nil {
		t.Fatalf("handleSearchObject error: %v", err)
	}
	if res.IsError {
		t.Fatalf("handleSearchObject returned an error result: %+v", res)
	}

	if searchQuery == "" {
		t.Fatal("no search request reached the server")
	}
	q, err := url.ParseQuery(searchQuery)
	if err != nil {
		t.Fatalf("parsing captured query %q: %v", searchQuery, err)
	}
	if got := q.Get("objectType"); got != "CLAS/OC" {
		t.Errorf("objectType = %q, want %q (short form should be canonicalized)", got, "CLAS/OC")
	}
	// Six, not five: upstream over-fetches by one so a full page can be told
	// apart from a page that happens to be exactly the size of the limit. The
	// point of this assertion is that the caller's limit still reaches the
	// server at all — the server-side filter is useless if maxResults is
	// dropped or fixed to a default on the way.
	if got := q.Get("maxResults"); got != "6" {
		t.Errorf("maxResults = %q, want %q (asked for 5, plus one truncation probe)", got, "6")
	}
	if got := q.Get("query"); got != "Z*" {
		t.Errorf("query = %q, want %q", got, "Z*")
	}
}
