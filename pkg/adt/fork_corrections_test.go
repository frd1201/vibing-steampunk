package adt

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Characterisation tests for this fork's own corrections.
//
// Everything covered here is fork-only work that upstream does not carry, and
// that `go test ./...` did not exercise before this file existed — a merge that
// dropped any of it would have stayed green. They are the acceptance criteria
// for an upstream sync: green before the merge, green after it.
//
// They are deliberately written against observable behaviour (the URL that goes
// out, the type that comes back) rather than against a particular internal
// shape, so they survive upstream restructuring the implementation underneath.
//
// This file has no upstream counterpart, so it cannot itself land in a merge
// conflict.

// emptyCheckRunXML is a syntax-check response with no findings — enough for the
// parser, and it keeps these tests about routing rather than about diagnostics.
const emptyCheckRunXML = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">` +
	`<chkrun:checkReport><chkrun:checkMessageList/></chkrun:checkReport>` +
	`</chkrun:checkRunReports>`

// forkTestResponse builds a mock response whose CSRF header actually survives
// http.Header.Get. The shared newWorkflowTestResponse helper writes the header
// with a non-canonical map key ("X-CSRF-Token" rather than "X-Csrf-Token"), so
// Get returns empty and every CSRF-bearing request in a test using it fails.
// Existing tests do not notice because they only exercise GETs.
func forkTestResponse(body string) *http.Response {
	h := http.Header{}
	h.Set("X-CSRF-Token", "test-token")
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     h,
	}
}

// --------------------------------------------------------------------------
// INCL (PROG/I) write support — fork PR #121
// --------------------------------------------------------------------------

// TestWriteInclude_UsesProgramIncludesURL pins the ADT endpoint for a program
// include. Upstream only ever reads includes; the write path is ours, and the
// /programs/includes/ segment is what distinguishes it from a plain program.
func TestWriteInclude_UsesProgramIncludesURL(t *testing.T) {
	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/includes/ZDEMO_INC": forkTestResponse(
				`<?xml version="1.0"?><asx:abap xmlns:asx="http://www.sap.com/abapxml">` +
					`<asx:values><DATA><LOCK_HANDLE>LH-1</LOCK_HANDLE>` +
					`<MODIFICATION_SUPPORT>NoModification</MODIFICATION_SUPPORT>` +
					`</DATA></asx:values></asx:abap>`),
			"/sap/bc/adt/programs/includes/ZDEMO_INC/source/main": forkTestResponse("OK"),
			"/sap/bc/adt/checkruns":                               forkTestResponse(emptyCheckRunXML),
			"/sap/bc/adt/activation":                              forkTestResponse("OK"),
			"discovery":                                           forkTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	if _, err := client.WriteInclude(context.Background(), "zdemo_inc", "* source", ""); err != nil {
		t.Fatalf("WriteInclude failed: %v", err)
	}

	// The include name must be upper-cased into the URL, and the source must go
	// to .../source/main rather than to the object URL itself.
	var sawObject, sawSource bool
	for _, req := range mock.requests {
		switch req.URL.Path {
		case "/sap/bc/adt/programs/includes/ZDEMO_INC":
			sawObject = true
		case "/sap/bc/adt/programs/includes/ZDEMO_INC/source/main":
			sawSource = true
		}
	}
	if !sawObject {
		t.Error("no request to /sap/bc/adt/programs/includes/ZDEMO_INC — include URL routing lost")
	}
	if !sawSource {
		t.Error("no request to .../ZDEMO_INC/source/main — include source write lost")
	}
}

// TestWriteSource_AcceptsINCLType guards the INCL arm of WriteSource's type
// whitelist. Both sides of the upstream merge extend that same switch — we
// added INCL, upstream added TABL — so the arm is easy to lose while resolving
// the conflict. Losing it makes every include write fail as an unsupported type.
func TestWriteSource_AcceptsINCLType(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	newClient := func() *Client {
		mock := &mockWorkflowTransport{
			responses: map[string]*http.Response{
				"discovery": forkTestResponse("OK"),
			},
		}
		return NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
	}

	res, err := newClient().WriteSource(context.Background(), "INCL", "ZDEMO_INC", "* source",
		&WriteSourceOptions{Mode: WriteModeUpdate})
	if err != nil {
		t.Fatalf("WriteSource(INCL) failed: %v", err)
	}
	if strings.Contains(res.Message, "Unsupported object type") {
		t.Errorf("INCL was rejected as an unsupported type — the INCL arm of the "+
			"whitelist is gone; message was: %s", res.Message)
	}

	// Guard against passing vacuously: a genuinely unknown type must still be
	// rejected, otherwise the assertion above proves nothing.
	res, err = newClient().WriteSource(context.Background(), "ZZZZ", "ZDEMO_INC", "* source",
		&WriteSourceOptions{Mode: WriteModeUpdate})
	if err != nil {
		t.Fatalf("WriteSource(ZZZZ) failed: %v", err)
	}
	if !strings.Contains(res.Message, "Unsupported object type") {
		t.Errorf("an unknown object type was not rejected; message was: %s", res.Message)
	}
}

// TestParseABAPFile_Include covers the .incl.abap extension. The name is derived
// from the filename (there is no INCLUDE statement to read it from), and '#'
// maps back to '/' for namespaced includes.
func TestParseABAPFile_Include(t *testing.T) {
	for _, tc := range []struct {
		filename string
		wantName string
	}{
		{"zdemo_inc.incl.abap", "ZDEMO_INC"},
		{"#zdemo#top.incl.abap", "/ZDEMO/TOP"},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.filename)
			if err := os.WriteFile(path, []byte("* include source\n"), 0644); err != nil {
				t.Fatal(err)
			}

			info, err := ParseABAPFile(path)
			if err != nil {
				t.Fatalf("ParseABAPFile failed: %v", err)
			}
			if info.ObjectType != ObjectTypeInclude {
				t.Errorf("ObjectType = %s, want %s", info.ObjectType, ObjectTypeInclude)
			}
			if info.ObjectName != tc.wantName {
				t.Errorf("ObjectName = %s, want %s", info.ObjectName, tc.wantName)
			}
		})
	}
}

// TestSyntaxCheck_ProgramIncludeGetsSourceMain guards the narrowing of
// isClassInclude: a *program* include carries /includes/ in its URL too, and
// before the fix it was mistaken for a class include and lost its /source/main
// suffix.
func TestSyntaxCheck_ProgramIncludeGetsSourceMain(t *testing.T) {
	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/checkruns": forkTestResponse(emptyCheckRunXML),
			"discovery":             forkTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	if _, err := client.SyntaxCheck(context.Background(),
		"/sap/bc/adt/programs/includes/ZDEMO_INC", "* source"); err != nil {
		t.Fatalf("SyntaxCheck failed: %v", err)
	}

	var body string
	for _, req := range mock.requests {
		if strings.Contains(req.URL.Path, "checkruns") && req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			body = string(b)
		}
	}
	if body == "" {
		t.Fatal("no checkruns request body recorded")
	}
	if !strings.Contains(body, "/programs/includes/ZDEMO_INC/source/main") {
		t.Errorf("artifactURI lacks /source/main for a program include; body was:\n%s", body)
	}
}

// --------------------------------------------------------------------------
// Secure-cookie stripping over plain HTTP — fork PR #120
// --------------------------------------------------------------------------

// TestNewHTTPClient_JarStripsSecureOverHTTP covers httpCookieJar. Without it a
// SAP system behind a plain-HTTP reverse proxy that still marks its cookies
// Secure loses the session on every subsequent request, which surfaces as
// permanently expired CSRF tokens.
func TestNewHTTPClient_JarStripsSecureOverHTTP(t *testing.T) {
	cfg := NewConfig("http://sap.example.local:8000", "user", "pass")
	client := cfg.NewHTTPClient()

	u, err := url.Parse("http://sap.example.local:8000/sap/bc/adt/discovery")
	if err != nil {
		t.Fatal(err)
	}

	client.Jar.SetCookies(u, []*http.Cookie{
		{Name: "SAP_SESSIONID_DEV_001", Value: "abc", Secure: true},
	})

	got := client.Jar.Cookies(u)
	if len(got) == 0 {
		t.Fatal("Secure cookie was dropped on a plain-HTTP URL — session would be lost")
	}
	if got[0].Name != "SAP_SESSIONID_DEV_001" {
		t.Errorf("unexpected cookie %q", got[0].Name)
	}
}

// TestNewHTTPClient_JarKeepsSecureOverHTTPS is the other half: over HTTPS the
// flag must be left alone. Stripping it everywhere would be a security
// regression rather than a proxy workaround.
func TestNewHTTPClient_JarKeepsSecureOverHTTPS(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := cfg.NewHTTPClient()

	secureURL, _ := url.Parse("https://sap.example.com:44300/sap/bc/adt/discovery")
	plainURL, _ := url.Parse("http://sap.example.com:44300/sap/bc/adt/discovery")

	client.Jar.SetCookies(secureURL, []*http.Cookie{
		{Name: "SAP_SESSIONID_DEV_001", Value: "abc", Secure: true},
	})

	if len(client.Jar.Cookies(secureURL)) == 0 {
		t.Error("cookie set over HTTPS is not returned over HTTPS")
	}
	if len(client.Jar.Cookies(plainURL)) != 0 {
		t.Error("a Secure cookie stored over HTTPS leaked to a plain-HTTP request")
	}
}

// TestSessionRecovery_PreservesSecureStrippingJar is the regression this fork
// already carries and that an upstream sync makes more likely to fire: the
// session-recovery paths rebuild the cookie jar, and a plain cookiejar.New(nil)
// silently discards the httpCookieJar wrapper. After any jar reset the
// stripping behaviour must still hold.
func TestSessionRecovery_PreservesSecureStrippingJar(t *testing.T) {
	cfg := NewConfig("http://sap.example.local:8000", "user", "pass")
	transport := NewTransport(cfg)

	transport.clearSAPSessionCookies()

	if transport.jar == nil {
		t.Fatal("no cookie jar after session reset")
	}

	u, _ := url.Parse("http://sap.example.local:8000/sap/bc/adt/discovery")
	transport.jar.SetCookies(u, []*http.Cookie{
		{Name: "SAP_SESSIONID_DEV_001", Value: "abc", Secure: true},
	})

	if len(transport.jar.Cookies(u)) == 0 {
		t.Error("after a session reset the jar no longer strips Secure over HTTP — " +
			"the reset rebuilt a plain cookiejar and dropped the httpCookieJar wrapper")
	}
}

// --------------------------------------------------------------------------
// CSRF HEAD -> GET fallback — fork PR #120
// --------------------------------------------------------------------------

// TestFetchCSRFToken_FallsBackToGET is a capability test, not an
// implementation test: older releases (BASIS 740, ECC EhP7) answer the HEAD
// probe without a token, and the client has to try GET before giving up.
//
// It deliberately says nothing about how a 403 on the HEAD is treated. Upstream
// removed that short-circuit on purpose — some systems refuse the HEAD and
// answer the GET perfectly well — so asserting it here would pin behaviour this
// fork intends to drop.
func TestFetchCSRFToken_FallsBackToGET(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			// HEAD on discovery: no token at all.
			newMockResponse(200, "", nil),
			// GET on discovery: token.
			newMockResponse(200, "<discovery/>", map[string]string{"X-CSRF-Token": "tok-from-get"}),
			// The actual request.
			newMockResponse(200, "OK", nil),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)

	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", &RequestOptions{
		Method: http.MethodPost,
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	var sawHead, sawGet bool
	for _, req := range mock.requests {
		if !strings.Contains(req.URL.Path, "discovery") {
			continue
		}
		switch req.Method {
		case http.MethodHead:
			sawHead = true
		case http.MethodGet:
			sawGet = true
		}
	}
	if !sawHead {
		t.Error("no HEAD probe on discovery — the fast path is gone")
	}
	if !sawGet {
		t.Error("no GET fallback after a tokenless HEAD — vsp would be unusable " +
			"against systems that answer HEAD without a CSRF token")
	}
}
