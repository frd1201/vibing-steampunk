package adt

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// listenOn404Port listens on a loopback port whose number contains "404",
// the shape that once turned a timeout into "package does not exist".
func listenOn404Port(t *testing.T) net.Listener {
	t.Helper()
	for p := 40400; p < 40500; p++ {
		if l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			return l
		}
	}
	t.Skip("no free loopback port in 40400-40499")
	return nil
}

// packageExists decides "missing" only from SAP's own answer: a 404, or "not
// found" in SAP's message. A failure that never got an answer is optimistic,
// whatever its text says.
func TestPackageExistsReadsSAPsAnswerNotTheErrorText(t *testing.T) {
	serve := func(t *testing.T, l net.Listener, h http.HandlerFunc) *Client {
		t.Helper()
		ts := httptest.NewUnstartedServer(h)
		if l != nil {
			_ = ts.Listener.Close()
			ts.Listener = l
		}
		ts.Start()
		t.Cleanup(ts.Close)
		return NewClient(ts.URL, "u", "p", WithClient("001"))
	}
	answer := func(status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-CSRF-Token", "t")
			if r.Method == http.MethodHead || r.Method == http.MethodGet {
				return
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}
	}

	t.Run("a timeout on a 404 port is not a missing package", func(t *testing.T) {
		c := serve(t, listenOn404Port(t), func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(5 * time.Second):
			case <-r.Context().Done():
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if !c.packageExists(ctx, "$TMP") {
			t.Fatal("a call that timed out was read as \"package does not exist\"")
		}
	})
	t.Run("SAP 404 is a missing package", func(t *testing.T) {
		c := serve(t, nil, answer(http.StatusNotFound, ""))
		if c.packageExists(context.Background(), "$ZNOPE") {
			t.Fatal("SAP's 404 was not read as a missing package")
		}
	})
	t.Run("SAP saying not found is a missing package", func(t *testing.T) {
		c := serve(t, nil, answer(http.StatusBadRequest, "Package $ZNOPE not found"))
		if c.packageExists(context.Background(), "$ZNOPE") {
			t.Fatal("SAP's \"not found\" was not read as a missing package")
		}
	})
	t.Run("SAP 500 is not a missing package", func(t *testing.T) {
		c := serve(t, nil, answer(http.StatusInternalServerError, "dump"))
		if !c.packageExists(context.Background(), "$TMP") {
			t.Fatal("a 500 was read as a missing package")
		}
	})
}
