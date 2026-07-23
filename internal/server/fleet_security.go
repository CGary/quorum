package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// fleetSecurityConfig carries the bind-derived state guardFleetToggle needs
// to evaluate one request. It is populated from Server fields by
// (*Server).fleetSecurityConfig (fleet_handlers.go) -- this file stays a pure,
// table-testable layer with no *Server dependency.
type fleetSecurityConfig struct {
	bindHost     string
	bindPort     int
	fleetToken   string
	loopbackBind bool
}

// newFleetToken generates a random 32-byte hex token via crypto/rand. It is
// called from Server.Start only for non-loopback binds; the token is
// delivered out-of-band via the server log, never embedded in the served
// page for a non-loopback bind.
func newFleetToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// isLoopbackHost reports whether a --host value binds only to the local
// machine: empty (Start's own 127.0.0.1 default applies before this is
// called elsewhere), "localhost", or a loopback IP literal.
func isLoopbackHost(host string) bool {
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isWildcardHost reports whether bindHost is an all-interfaces wildcard bind,
// for which an exact Host allowlist is impossible to enumerate.
func isWildcardHost(bindHost string) bool {
	switch bindHost {
	case "0.0.0.0", "::", "[::]":
		return true
	default:
		return false
	}
}

// allowedHostSet returns the exact lowercase "host:port" (and bare host when
// bindPort is the default HTTP port 80) values accepted as a Host header for
// a concrete, non-wildcard bind. Loopback binds additionally accept the
// common loopback aliases so both --host 127.0.0.1 and --host localhost
// deployments work.
func allowedHostSet(bindHost string, bindPort int) map[string]bool {
	set := map[string]bool{}
	portStr := strconv.Itoa(bindPort)
	add := func(h string) {
		h = strings.ToLower(h)
		set[net.JoinHostPort(h, portStr)] = true
		if bindPort == 80 {
			set[h] = true
		}
	}
	add(bindHost)
	if isLoopbackHost(bindHost) {
		add("127.0.0.1")
		add("localhost")
		add("::1")
	}
	return set
}

// hostHostname strips a possible port and IPv6 brackets from a
// Host-header-shaped or URL-authority-shaped string.
func hostHostname(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if h, _, err := net.SplitHostPort(raw); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(raw, "[]")
}

// hostMatchesBind reports whether a Host-header-shaped value (host[:port])
// is acceptable for the configured bind. An empty cfg.bindHost means the
// Server was never started via Start (e.g. a bare &Server{} in a test or an
// unconfigured caller); it is treated permissively so pre-existing
// zero-value-Server tests keep working without asserting a Host allowlist
// that was never established.
//
// For a concrete bind, the Host header must match allowedHostSet exactly.
// For a wildcard bind (0.0.0.0 / ::), an exact allowlist is impossible: any
// IP-literal Host or the "localhost" name is accepted, and unlisted DNS
// names are rejected -- this blocks classic DNS-rebinding attacks (which
// require a resolvable attacker-controlled domain) while still permitting
// direct-IP LAN access.
func hostMatchesBind(hostHeader string, cfg fleetSecurityConfig) bool {
	if cfg.bindHost == "" {
		return true
	}
	if hostHeader == "" {
		return false
	}
	if isWildcardHost(cfg.bindHost) {
		h := hostHostname(hostHeader)
		if h == "" {
			return false
		}
		if net.ParseIP(h) != nil {
			return true
		}
		return h == "localhost"
	}
	return allowedHostSet(cfg.bindHost, cfg.bindPort)[strings.ToLower(hostHeader)]
}

// sameOriginOK enforces a same-origin policy via the Origin header, falling
// back to Referer when Origin is absent. A request carrying neither header
// is treated as same-origin: browsers always attach at least one of them to
// a cross-origin POST (including the "simple request" text/plain trick that
// skips CORS preflight), so their total absence identifies a non-browser
// client (CLI/curl) and preserves CLI/curl parity with the dashboard UI.
func sameOriginOK(r *http.Request, cfg fleetSecurityConfig) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return hostMatchesBind(u.Host, cfg)
}

// guardFleetToggle is the single entry point fleetToggleHandler calls before
// any toggle logic runs. It enforces, in order:
//  1. Content-Type: application/json -- independently defeats the
//     text/plain "simple request" CSRF trick, which never triggers a CORS
//     preflight and so could otherwise carry an arbitrary cross-origin body.
//  2. Same-origin, via sameOriginOK.
//  3. Host allowlist, via hostMatchesBind (DNS-rebinding defense).
//  4. X-Quorum-Fleet-Token, required only when cfg.loopbackBind is false.
//
// It never calls core.DisableFleetTarget/EnableFleetTarget or otherwise
// mutates state -- it only decides whether the caller may proceed.
func guardFleetToggle(r *http.Request, cfg fleetSecurityConfig) (ok bool, status int, message string) {
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if !strings.HasPrefix(ct, "application/json") {
		return false, http.StatusForbidden, "Content-Type must be application/json"
	}

	if !sameOriginOK(r, cfg) {
		return false, http.StatusForbidden, "cross-origin request rejected"
	}

	if !hostMatchesBind(r.Host, cfg) {
		return false, http.StatusForbidden, "unrecognized Host header"
	}

	if !cfg.loopbackBind {
		token := r.Header.Get("X-Quorum-Fleet-Token")
		// Constant-time compare: the bearer token is a secret, so a plain
		// != comparison would leak a timing side-channel proportional to
		// the shared-prefix length.
		if token == "" || cfg.fleetToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(cfg.fleetToken)) != 1 {
			return false, http.StatusUnauthorized, "missing or invalid X-Quorum-Fleet-Token"
		}
	}

	return true, http.StatusOK, ""
}
