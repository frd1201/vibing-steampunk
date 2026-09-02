package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// PackageExists is the only thing standing between the MCP installer and the
// create path: handleInstallZADTVSP used to test GetPackage's URI field, which
// nothing populates, so it created the package on every run. Nothing covered
// the probe that replaced it.
func TestPackageExists(t *testing.T) {
	tests := []struct {
		name       string
		pkg        string
		status     int
		wantExists bool
		wantErr    bool
	}{
		{name: "200 means the package is there", pkg: "$ZDEMO_PKG", status: http.StatusOK, wantExists: true},
		{name: "404 means it is not", pkg: "$ZDEMO_MISSING", status: http.StatusNotFound},
		{
			// Anything that is not a clean 404 must reach the caller as an
			// error. Classifying a 500 or an auth failure as "missing" is how
			// an installer ends up creating a package that already exists.
			name:    "500 is inconclusive, not missing",
			pkg:     "$ZDEMO_PKG",
			status:  http.StatusInternalServerError,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/discovery") {
					w.Header().Set("X-CSRF-Token", "TOKEN")
					return
				}
				gotPath = r.URL.Path
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "TESTUSER", "secret")
			exists, err := client.PackageExists(context.Background(), tt.pkg)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for HTTP %d, got exists=%v and nil", tt.status, exists)
				}
				if exists {
					t.Errorf("an inconclusive probe must not report the package as existing")
				}
				return
			}
			if err != nil {
				t.Fatalf("PackageExists: %v", err)
			}
			if exists != tt.wantExists {
				t.Errorf("PackageExists(%q) = %v, want %v", tt.pkg, exists, tt.wantExists)
			}
			if want := "/sap/bc/adt/packages/"; !strings.HasPrefix(gotPath, want) {
				t.Errorf("probed %q, expected the %s resource", gotPath, want)
			}
		})
	}
}

// An empty name is a caller mistake, and answering "false" would send an
// installer straight into creating a package with no name.
func TestPackageExistsRefusesEmptyName(t *testing.T) {
	client := NewClient("https://sap.example.com:44300", "TESTUSER", "secret")
	if _, err := client.PackageExists(context.Background(), ""); err == nil {
		t.Fatal("expected an error for an empty package name")
	}
}
