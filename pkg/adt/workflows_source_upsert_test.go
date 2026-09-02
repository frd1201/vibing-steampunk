package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Upsert picks between update and create by asking whether the object is
// there. What it used to do with the answer was `objectExists = (err == nil)`,
// which files every failure — a timeout, a 500, a host that will not resolve —
// as "not there", and then creates.
//
// These three cases are the whole distinction: a definite no, a definite yes,
// and no answer at all. The third is the one that was wrong, and it is wrong in
// the expensive direction: an edit to an existing class becomes an attempt to
// create one.
func TestUpsertDistinguishesAbsentFromUnanswered(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"a definite no leads to create", http.StatusNotFound, "Package is required"},
		{"no answer refuses to guess", http.StatusInternalServerError, "cannot tell whether"},
		{"a definite yes leads to update", http.StatusOK, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Only the existence probe is answered with the case's status;
				// the update path beyond it is allowed to fail however it
				// likes, because what is under test is the decision.
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/oo/classes/") {
					w.WriteHeader(tc.status)
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "user", "pass")
			result, err := client.WriteSource(context.Background(), "CLAS", "ZCL_DEMO",
				"CLASS zcl_demo DEFINITION.\nENDCLASS.",
				&WriteSourceOptions{Mode: WriteModeUpsert})
			if err != nil {
				t.Fatalf("WriteSource returned an error rather than a result: %v", err)
			}

			switch tc.want {
			case "":
				// A yes must not reach the create branch. What the update
				// attempt then does against a server answering 500 is not this
				// test's business; that it did not try to create is.
				if strings.Contains(result.Message, "Package is required") ||
					strings.Contains(result.Message, "cannot tell whether") {
					t.Errorf("an existing object was not treated as existing: %q", result.Message)
				}
			default:
				if !strings.Contains(result.Message, tc.want) {
					t.Errorf("got %q, expected it to contain %q", result.Message, tc.want)
				}
			}
		})
	}
}

// The fix is only worth having if it holds for every type that has a probe,
// and there are eight of them. One case per type, because the switch is eight
// separate lines and a fix applied to seven of them is the same defect.
//
// FUNC is the ninth writable type and is absent here on purpose: WriteSource
// resolves a function module's group before it gets anywhere near the existence
// check, so a probe failure is not a state it can reach.
func TestNoAnswerRefusesToGuessForEveryProbedType(t *testing.T) {
	for _, objType := range []string{"PROG", "CLAS", "INTF", "DDLS", "BDEF", "SRVD", "SRVB", "TABL"} {
		t.Run(objType, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "user", "pass")
			result, err := client.WriteSource(context.Background(), objType, "ZDEMO_OBJ", "* source",
				&WriteSourceOptions{Mode: WriteModeUpsert})
			if err != nil {
				t.Fatalf("WriteSource: %v", err)
			}
			if !strings.Contains(result.Message, "cannot tell whether") {
				t.Errorf("%s: a server that answered nothing was read as a verdict: %q",
					objType, result.Message)
			}
		})
	}
}

// The silent-success class: WriteSource answers a syntax error, a failed
// activation or an unsupported type with (result{Success:false}, nil). Three
// deploy loops read only the error and printed "OK" for objects that were
// never written; Deployed is what they ask now.
func TestWriteSourceResultDeployed(t *testing.T) {
	tests := []struct {
		name    string
		result  *WriteSourceResult
		wantOK  bool
		wantMsg string
	}{
		{
			name:    "a refusal with a reason keeps the reason",
			result:  &WriteSourceResult{Success: false, Message: "Source has syntax errors - not saved"},
			wantMsg: "Source has syntax errors - not saved",
		},
		{
			name:    "a refusal with no reason still reads as a failure",
			result:  &WriteSourceResult{Success: false},
			wantMsg: "unknown failure",
		},
		{
			// (nil, nil) must not be mistaken for a success by a caller that
			// only nil-checks the error.
			name:    "a nil result is a failure, not a success",
			result:  nil,
			wantMsg: "unknown failure",
		},
		{
			name:    "a success is a success",
			result:  &WriteSourceResult{Success: true, Message: "Program updated and activated successfully"},
			wantOK:  true,
			wantMsg: "Program updated and activated successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.result.Deployed()
			if ok != tt.wantOK {
				t.Errorf("Deployed() ok = %v, want %v", ok, tt.wantOK)
			}
			if msg != tt.wantMsg {
				t.Errorf("Deployed() msg = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}
