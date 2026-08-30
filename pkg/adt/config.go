// Package adt provides a Go client for SAP ABAP Development Tools (ADT) REST API.
package adt

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// SessionType defines how the client manages server sessions.
type SessionType string

const (
	// SessionStateful maintains a server session via sap-contextid cookie.
	SessionStateful SessionType = "stateful"
	// SessionStateless does not persist sessions.
	SessionStateless SessionType = "stateless"
	// SessionKeep uses existing session if available, otherwise stateless.
	SessionKeep SessionType = "keep"
)

// ParseSessionType turns a SAP_SESSION_TYPE value into a SessionType, reporting
// whether it was one of the three known names. Surrounding whitespace and case
// are ignored; an empty value is not an error, it simply means "unset" and
// returns ok == false with the zero SessionType.
//
// One parser, because there were two: the MCP server compared the raw string
// while the CLI lower-cased and trimmed it, so SAP_SESSION_TYPE=Stateful — or a
// value with a trailing space out of a .env file — took effect on one side of
// the same product and was warned away on the other.
func ParseSessionType(v string) (SessionType, bool) {
	switch st := SessionType(strings.ToLower(strings.TrimSpace(v))); st {
	case SessionStateful, SessionStateless, SessionKeep:
		return st, true
	default:
		return "", false
	}
}

// Config holds the configuration for an ADT client connection.
type Config struct {
	// BaseURL is the SAP system URL (e.g., "https://vhcalnplci.dummy.nodomain:44300")
	BaseURL string
	// Username for SAP authentication
	Username string
	// Password for SAP authentication
	Password string
	// Client is the SAP client number (e.g., "001")
	Client string
	// Language for SAP session (e.g., "EN")
	Language string
	// InsecureSkipVerify disables TLS certificate verification
	InsecureSkipVerify bool
	// SessionType defines session management behavior
	SessionType SessionType
	// Timeout for HTTP requests
	Timeout time.Duration
	// Cookies for cookie-based authentication (alternative to basic auth)
	Cookies map[string]string
	// Verbose enables verbose logging
	Verbose bool
	// Safety defines protection parameters to prevent unintended modifications
	Safety SafetyConfig
	// Features controls optional feature detection and enablement
	Features FeatureConfig
	// TerminalID for debugger session (shared with SAP GUI for cross-tool debugging)
	TerminalID string

	// ReauthFunc is called on 401 to re-authenticate (e.g., re-run SAML dance).
	// Returns fresh cookies for the SAP system. Only used when HasBasicAuth() is false.
	ReauthFunc func(ctx context.Context) (map[string]string, error)

	// ReauthTimeout caps one re-authentication attempt. Zero uses the default,
	// which suits a re-auth that runs unattended. Raise it where the flow may
	// stop to ask a human something — a browser sign-in with a second factor
	// takes far longer than any machine-to-machine handshake.
	ReauthTimeout time.Duration
}

// Option is a functional option for configuring the ADT client.
type Option func(*Config)

// WithClient sets the SAP client number.
func WithClient(client string) Option {
	return func(c *Config) {
		c.Client = client
	}
}

// WithLanguage sets the SAP session language.
func WithLanguage(lang string) Option {
	return func(c *Config) {
		c.Language = lang
	}
}

// WithInsecureSkipVerify disables TLS certificate verification.
func WithInsecureSkipVerify() Option {
	return func(c *Config) {
		c.InsecureSkipVerify = true
	}
}

// WithSessionType sets the session management behavior.
func WithSessionType(st SessionType) Option {
	return func(c *Config) {
		c.SessionType = st
	}
}

// WithTimeout sets the HTTP request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}

// WithCookies sets cookies for cookie-based authentication.
func WithCookies(cookies map[string]string) Option {
	return func(c *Config) {
		c.Cookies = cookies
	}
}

// WithVerbose enables verbose logging.
func WithVerbose() Option {
	return func(c *Config) {
		c.Verbose = true
	}
}

// WithSafety sets the safety configuration.
func WithSafety(safety SafetyConfig) Option {
	return func(c *Config) {
		c.Safety = safety
	}
}

// WithReadOnly enables read-only mode (blocks all write operations).
func WithReadOnly() Option {
	return func(c *Config) {
		c.Safety.ReadOnly = true
	}
}

// WithBlockFreeSQL blocks execution of arbitrary SQL queries.
func WithBlockFreeSQL() Option {
	return func(c *Config) {
		c.Safety.BlockFreeSQL = true
	}
}

// WithAllowedPackages restricts operations to specific packages.
func WithAllowedPackages(packages ...string) Option {
	return func(c *Config) {
		c.Safety.AllowedPackages = packages
	}
}

// WithEnableTransports enables transport management operations.
// By default, transport operations are disabled - this flag explicitly enables them.
func WithEnableTransports() Option {
	return func(c *Config) {
		c.Safety.EnableTransports = true
	}
}

// WithTransportReadOnly allows only read operations on transports (list, get).
// Create, release, delete operations will be blocked.
func WithTransportReadOnly() Option {
	return func(c *Config) {
		c.Safety.TransportReadOnly = true
	}
}

// WithAllowedTransports restricts transport operations to specific transports.
// Supports wildcards: "A4HK*" matches all transports starting with A4HK.
func WithAllowedTransports(transports ...string) Option {
	return func(c *Config) {
		c.Safety.AllowedTransports = transports
	}
}

// WithAllowTransportableEdits enables editing objects that require transport requests.
// By default, only local objects ($TMP, $* packages) can be edited.
// When enabled, users can provide transport parameters to EditSource/WriteSource.
// WARNING: This allows modifications to non-local objects that may affect production systems.
func WithAllowTransportableEdits() Option {
	return func(c *Config) {
		c.Safety.AllowTransportableEdits = true
	}
}

// HasBasicAuth returns true if username and password are configured.
func (c *Config) HasBasicAuth() bool {
	return c.Username != "" && c.Password != ""
}

// HasCookieAuth returns true if cookies are configured.
func (c *Config) HasCookieAuth() bool {
	return len(c.Cookies) > 0
}

// NewConfig creates a new Config with the given base URL, username, password,
// and optional configuration options.
func NewConfig(baseURL, username, password string, opts ...Option) *Config {
	cfg := &Config{
		BaseURL:     baseURL,
		Username:    username,
		Password:    password,
		Client:      "001",
		Language:    "EN",
		SessionType: SessionStateless,
		Timeout:     60 * time.Second,
		Safety:      UnrestrictedSafetyConfig(), // Default: no restrictions for backwards compatibility
		Features:    DefaultFeatureConfig(),     // Default: auto-detect all features
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// WithFeatures sets the feature configuration.
func WithFeatures(features FeatureConfig) Option {
	return func(c *Config) {
		c.Features = features
	}
}

// WithReauthFunc sets the re-authentication function for 401 recovery.
// Used by SAML auth to re-run the SAML dance when the session expires.
func WithReauthFunc(f func(ctx context.Context) (map[string]string, error)) Option {
	return func(c *Config) {
		c.ReauthFunc = f
	}
}

// WithReauthTimeout caps a single re-authentication attempt.
func WithReauthTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.ReauthTimeout = d
	}
}

// WithTerminalID sets the debugger terminal ID.
// Use the same ID as SAP GUI to enable cross-tool breakpoint sharing.
// SAP GUI stores this in: Windows Registry HKCU\Software\SAP\ABAP Debugging\TerminalID
// or on Linux/Mac: ~/.SAP/ABAPDebugging/terminalId
func WithTerminalID(terminalID string) Option {
	return func(c *Config) {
		c.TerminalID = terminalID
	}
}

// httpCookieJar wraps cookiejar.Jar and strips the Secure flag when storing cookies
// received over plain HTTP. This is required for SAP systems accessed via HTTP reverse
// proxies that still set Secure cookies (e.g. nginx in front of SAP ICM). Go's standard
// jar won't send Secure cookies on HTTP requests, causing CSRF tokens to appear expired
// because the session cookie never reaches the server on subsequent requests.
type httpCookieJar struct {
	*cookiejar.Jar
}

func (j *httpCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if u.Scheme == "http" {
		stripped := make([]*http.Cookie, len(cookies))
		for i, c := range cookies {
			copy := *c
			copy.Secure = false
			stripped[i] = &copy
		}
		cookies = stripped
	}
	j.Jar.SetCookies(u, cookies)
}

// newCookieJar builds the cookie jar every code path must use. Going through
// here rather than calling cookiejar.New directly is what keeps the Secure
// stripping alive across a session reset — a plain jar built at a recovery site
// silently drops the wrapper and reintroduces the lost-session bug on
// plain-HTTP systems.
func newCookieJar() http.CookieJar {
	base, err := cookiejar.New(nil)
	if err != nil {
		return nil
	}
	return &httpCookieJar{base}
}

// NewHTTPClient creates an http.Client configured for the given Config.
func (c *Config) NewHTTPClient() *http.Client {
	jar := newCookieJar()

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment, // Honor HTTP_PROXY/HTTPS_PROXY env vars
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.InsecureSkipVerify,
		},
	}

	client := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   c.Timeout,
	}

	// Preserve ADT-critical headers across redirects.
	//
	// Go's default strips Authorization / WWW-Authenticate / Cookie / Cookie2
	// on cross-origin redirects per RFC 7235 §4.2 — SAP BTP/Cloud SAML flows
	// need Authorization back, otherwise the IdP dance drops it and the user
	// gets 401 even though curl works (issue #90).
	//
	// Custom headers like X-CSRF-Token and X-sap-adt-sessiontype are *not*
	// in Go's sensitive-headers list, so Go technically forwards them by
	// default. We re-set them explicitly anyway for two reasons:
	//   - defensive: guards against any Go version or middleware tweak that
	//     decides to strip custom headers on its own;
	//   - intent: makes it obvious in the code that these two headers are
	//     load-bearing for the lock→write→unlock ADT sequence. If either
	//     goes missing across a redirect, the second hop hits SAP with a
	//     fresh (stateless) session-type or a missing CSRF token, and the
	//     lock handle / mutation is rejected.
	// Only for hops that stay on the SAP host. Re-attaching unconditionally
	// sent Basic credentials and the session CSRF token to whatever host the
	// chain led to — and an expired session on an SSO system leads to the
	// identity provider, which is why redirectedAwayFromSAP exists.
	//
	// Off-host the headers are DELETED, not merely left unset. Go's own
	// makeHeadersCopier runs before CheckRedirect and copies every header that
	// is not on its sensitive list — Authorization, Www-Authenticate, Cookie
	// and Cookie2 are stripped cross-origin, X-CSRF-Token and
	// X-sap-adt-sessiontype are not. So declining to *set* them here left the
	// session's CSRF token going to the identity provider exactly as before;
	// only an explicit Del actually stops it.
	//
	// The other half of that ordering is why the same-host branch is nearly a
	// no-op: Go already preserves Authorization for a same-host or subdomain
	// hop, so the re-attach only ever added anything cross-origin — which is
	// now refused. A BTP SAML flow that genuinely needs Authorization on a
	// foreign host (issue #90's abap → abap-web hop) therefore no longer gets
	// it, and would need an explicit, named allowance for that one host rather
	// than a blanket "any host in the chain".
	//
	// The comparison is on the *hostname*, case-folded — not on host:port.
	// Two reasons, and they pull the same way:
	//   - `==` on the raw host made an ICM redirect that merely changed the
	//     case of the FQDN, or spelled out :443, look foreign, and the headers
	//     this handler exists to preserve were dropped on an intra-SAP hop.
	//   - the Del below must not be stricter than Go's own rule, which ignores
	//     the port entirely (shouldCopyHeaderOnRedirect compares hostnames).
	//     A box that answers on 44300 and redirects to 8443 is one machine;
	//     deleting Basic credentials there would break a hop that worked
	//     before this handler existed.
	// redirectedAwayFromSAP (http.go) compares host:port with EqualFold, so it
	// is stricter on the port and identical on case; the difference only shows
	// on a port-changing hop, where this predicate is deliberately the looser
	// of the two.
	sapHostname := ""
	if u, err := url.Parse(c.BaseURL); err == nil {
		sapHostname = u.Hostname()
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if len(via) == 0 {
			return nil
		}
		// An unparseable or scheme-less BaseURL leaves sapHostname empty. Treat
		// that as "not the SAP host" so the credentials-off-host rule holds;
		// the alternative is to silently disable the whole handler.
		onSAPHost := sapHostname != "" &&
			strings.EqualFold(req.URL.Hostname(), sapHostname)
		if !onSAPHost {
			req.Header.Del("Authorization")
			req.Header.Del("X-CSRF-Token")
			req.Header.Del("X-sap-adt-sessiontype")
			return nil
		}
		first := via[0]
		if auth := first.Header.Get("Authorization"); auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if csrf := first.Header.Get("X-CSRF-Token"); csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if st := first.Header.Get("X-sap-adt-sessiontype"); st != "" {
			req.Header.Set("X-sap-adt-sessiontype", st)
		}
		return nil
	}

	return client
}
