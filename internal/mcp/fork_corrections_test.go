package mcp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// Characterisation tests for this fork's corrections on the MCP server side.
//
// `internal/mcp/server.go` is one of the files an upstream merge conflicts on,
// and the SAP_SESSION_TYPE block sits right next to code upstream rewrote. No
// test covered it before this file existed, so a resolution that dropped the
// block would have stayed green.
//
// This file has no upstream counterpart and so cannot itself conflict.

// captureStderr runs fn with os.Stderr redirected and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	os.Stderr = orig
	w.Close()
	out := <-done
	r.Close()
	return out
}

func newSessionTypeConfig(sessionType string) *Config {
	return &Config{
		BaseURL:     "https://sap.example.com:44300",
		Username:    "TESTUSER",
		Password:    "secret",
		Client:      "100",
		SessionType: sessionType,
	}
}

// TestNewServer_AcceptsKnownSessionTypes pins the SAP_SESSION_TYPE plumbing.
// The three values come from adt.SessionType; a value the server recognises
// must be passed through without complaint.
func TestNewServer_AcceptsKnownSessionTypes(t *testing.T) {
	for _, st := range []string{"stateful", "stateless", "keep"} {
		t.Run(st, func(t *testing.T) {
			out := captureStderr(t, func() {
				if srv := NewServer(newSessionTypeConfig(st)); srv == nil {
					t.Fatal("NewServer returned nil")
				}
			})
			if strings.Contains(out, "SAP_SESSION_TYPE") {
				t.Errorf("session type %q was rejected as unknown; stderr: %s", st, out)
			}
		})
	}
}

// TestNewServer_WarnsOnUnknownSessionType is the other half: without it the
// test above would still pass if the whole block were deleted, because a
// deleted block also prints nothing.
func TestNewServer_WarnsOnUnknownSessionType(t *testing.T) {
	out := captureStderr(t, func() {
		if srv := NewServer(newSessionTypeConfig("sideways")); srv == nil {
			t.Fatal("NewServer returned nil")
		}
	})

	if !strings.Contains(out, "SAP_SESSION_TYPE") {
		t.Errorf("an unknown session type produced no warning — the SAP_SESSION_TYPE "+
			"validation in NewServer is gone; stderr was: %q", out)
	}
}

// TestNewServer_EmptySessionTypeIsSilent guards the default path: leaving the
// variable unset is normal operation, not a misconfiguration.
func TestNewServer_EmptySessionTypeIsSilent(t *testing.T) {
	out := captureStderr(t, func() {
		if srv := NewServer(newSessionTypeConfig("")); srv == nil {
			t.Fatal("NewServer returned nil")
		}
	})

	if strings.Contains(out, "SAP_SESSION_TYPE") {
		t.Errorf("an unset session type warned; stderr: %s", out)
	}
}

// TestNewServer_SessionTypeReachesTheWire is the test that actually proves the
// wiring, rather than proving the validation around it: a merge resolution that
// kept the warning but dropped the opts.append would satisfy the tests above
// and still leave every request stateless.
//
// It drives a real request through the server's own ADT client and reads the
// header off the wire.
func TestNewServer_SessionTypeReachesTheWire(t *testing.T) {
	for _, tc := range []struct {
		sessionType string
		wantHeader  string
	}{
		{"stateful", "stateful"},
		{"stateless", "stateless"},
	} {
		t.Run(tc.sessionType, func(t *testing.T) {
			var mu sync.Mutex
			seen := map[string]string{}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				if v := r.Header.Get("X-sap-adt-sessiontype"); v != "" {
					seen[r.URL.Path] = v
				}
				mu.Unlock()
				w.Header().Set("X-CSRF-Token", "tok")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("REPORT zdemo."))
			}))
			defer srv.Close()

			cfg := newSessionTypeConfig(tc.sessionType)
			cfg.BaseURL = srv.URL

			s := NewServer(cfg)
			if s == nil {
				t.Fatal("NewServer returned nil")
			}

			if _, err := s.adtClient.GetProgram(context.Background(), "ZDEMO"); err != nil {
				t.Fatalf("GetProgram failed: %v", err)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(seen) == 0 {
				t.Fatal("no request carried an X-sap-adt-sessiontype header at all")
			}
			for path, got := range seen {
				if got != tc.wantHeader {
					t.Errorf("%s sent X-sap-adt-sessiontype=%q, want %q — "+
						"SAP_SESSION_TYPE did not reach the ADT client", path, got, tc.wantHeader)
				}
			}
		})
	}
}
