package adt

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// corrNr on the LOCK request. ADT takes the transport on the LOCK itself, and
// on-premise systems that bind the lock to a request expect it there. These
// tests pin it for LockObject and for the write paths that lock on their own.

// TestLockObject_EmitsCorrNr pins both directions: a supplied transport is on
// the LOCK, and without one the request is exactly what it was before.
func TestLockObject_EmitsCorrNr(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport []string
		want      string
	}{
		{"with transport", []string{"TR-EXAMPLE"}, "TR-EXAMPLE"},
		{"empty transport", []string{""}, ""},
		{"no transport argument", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &lockQueryRecorder{respond: func(*http.Request) (int, string) { return http.StatusOK, lockHandleXML }}
			if _, err := transportableEditClient(rec).LockObject(context.Background(),
				"/sap/bc/adt/oo/classes/zcl_demo", "MODIFY", tc.transport...); err != nil {
				t.Fatalf("LockObject: %v", err)
			}
			if len(rec.locks) != 1 {
				t.Fatalf("recorded %d LOCK requests, want 1", len(rec.locks))
			}
			if got, present := rec.locks[0].Get("corrNr"), rec.locks[0].Has("corrNr"); got != tc.want || present != (tc.want != "") {
				t.Errorf("LOCK corrNr = %q (present %v), want %q", got, present, tc.want)
			}
		})
	}
}

// lockQueryRecorder answers every request through respond and remembers the
// query of each LOCK. Bodies are built per call, so a document read twice —
// once before the lock and once under it — is served twice.
type lockQueryRecorder struct {
	respond func(r *http.Request) (int, string)
	locks   []url.Values
}

func (m *lockQueryRecorder) Do(r *http.Request) (*http.Response, error) {
	if r.URL.Query().Get("_action") == "LOCK" {
		m.locks = append(m.locks, r.URL.Query())
	}
	status, body := m.respond(r)
	h := http.Header{}
	h.Set("X-CSRF-Token", "test-token")
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: h}, nil
}

const lockHandleXML = `<?xml version="1.0"?><asx:abap xmlns:asx="http://www.sap.com/abapxml">` +
	`<asx:values><DATA><LOCK_HANDLE>LH-1</LOCK_HANDLE></DATA></asx:values></asx:abap>`

func transportableEditClient(doer HTTPDoer) *Client {
	safety := UnrestrictedSafetyConfig()
	safety.AllowTransportableEdits = true
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithSafety(safety))
	return NewClientWithTransport(cfg, NewTransportWithClient(cfg, doer))
}

func assertLockCarried(t *testing.T, locks []url.Values, want string) {
	t.Helper()
	if len(locks) == 0 {
		t.Fatal("no LOCK request was recorded")
	}
	for _, q := range locks {
		if got := q.Get("corrNr"); got != want {
			t.Errorf("LOCK carried corrNr=%q, want %q", got, want)
		}
	}
}

// TestSetDescription_PassesTransportToLock pins corrNr on the LOCK of the
// description write (#201), which takes its own lock.
func TestSetDescription_PassesTransportToLock(t *testing.T) {
	rec := &lockQueryRecorder{respond: func(r *http.Request) (int, string) {
		switch {
		case r.URL.Query().Get("_action") == "LOCK":
			return http.StatusOK, lockHandleXML
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/programs/programs/zdemo"):
			return http.StatusOK, `<program:abapProgram xmlns:adtcore="http://www.sap.com/adt/core" adtcore:description="Old"/>`
		default:
			return http.StatusOK, ""
		}
	}}

	if _, err := transportableEditClient(rec).SetDescription(context.Background(), "PROG", "ZDEMO", "", "New", "TR-EXAMPLE"); err != nil {
		t.Fatalf("SetDescription failed: %v", err)
	}
	assertLockCarried(t, rec.locks, "TR-EXAMPLE")
}

// TestWriteTextPool_PassesTransportToLock is the same pin for the text pool
// write (#200).
func TestWriteTextPool_PassesTransportToLock(t *testing.T) {
	rec := &lockQueryRecorder{respond: func(r *http.Request) (int, string) {
		switch {
		case r.URL.Query().Get("_action") == "LOCK":
			return http.StatusOK, lockHandleXML
		case strings.Contains(r.URL.Path, "/datapreview/"):
			// MasterLanguage reads TADIR; E is the master, so EN is no translation.
			return http.StatusOK, tableXML(xmlCol{name: "MASTERLANG", data: []string{"E"}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/textelements/"):
			return http.StatusOK, "001=Old\n"
		default:
			return http.StatusOK, ""
		}
	}}

	texts := map[string]map[string]string{"I": {"001": "New"}}
	_, err := transportableEditClient(rec).WriteTextPool(context.Background(),
		TextPoolTarget{Type: "CLAS", Name: "ZCL_DEMO"}, "EN", texts, "TR-EXAMPLE", TextPoolOptions{})
	if err != nil {
		t.Fatalf("WriteTextPool failed: %v", err)
	}
	assertLockCarried(t, rec.locks, "TR-EXAMPLE")
}
