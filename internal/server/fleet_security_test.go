package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"", true},
		{"localhost", true},
		{"LOCALHOST", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"::", false},
		{"192.168.1.5", false},
		{"example.com", false},
	}
	for _, c := range cases {
		if got := isLoopbackHost(c.host); got != c.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestAllowedHostSet(t *testing.T) {
	set := allowedHostSet("192.168.1.5", 8080)
	if !set["192.168.1.5:8080"] {
		t.Fatalf("expected bind host:port allowed, got %+v", set)
	}
	if set["localhost:8080"] {
		t.Fatalf("non-loopback bind must not accept localhost, got %+v", set)
	}

	loopSet := allowedHostSet("127.0.0.1", 4173)
	for _, h := range []string{"127.0.0.1:4173", "localhost:4173", "[::1]:4173"} {
		if !loopSet[h] {
			t.Errorf("expected loopback alias %q allowed, got %+v", h, loopSet)
		}
	}
}

func TestHostMatchesBind(t *testing.T) {
	cfg := fleetSecurityConfig{bindHost: "127.0.0.1", bindPort: 4173}
	if !hostMatchesBind("127.0.0.1:4173", cfg) {
		t.Error("expected exact bind host:port to match")
	}
	if !hostMatchesBind("localhost:4173", cfg) {
		t.Error("expected localhost alias to match a loopback bind")
	}
	if hostMatchesBind("evil.example.com:4173", cfg) {
		t.Error("expected unrelated Host header to be rejected")
	}

	wildcard := fleetSecurityConfig{bindHost: "0.0.0.0", bindPort: 4173}
	if !hostMatchesBind("192.168.1.9:4173", wildcard) {
		t.Error("expected IP-literal Host to be accepted on a wildcard bind")
	}
	if !hostMatchesBind("localhost:4173", wildcard) {
		t.Error("expected localhost to be accepted on a wildcard bind")
	}
	if hostMatchesBind("attacker.example.com:4173", wildcard) {
		t.Error("expected an unlisted DNS name to be rejected on a wildcard bind (DNS rebinding)")
	}

	zeroValue := fleetSecurityConfig{}
	if !hostMatchesBind("anything.example.com", zeroValue) {
		t.Error("expected an unconfigured (bindHost==\"\") config to be permissive")
	}
}

func TestSameOriginOK(t *testing.T) {
	cfg := fleetSecurityConfig{bindHost: "127.0.0.1", bindPort: 4173}

	req := httptest.NewRequest(http.MethodPost, "/api/fleet/toggle", nil)
	if !sameOriginOK(req, cfg) {
		t.Error("expected no Origin/Referer to be treated as same-origin (CLI/curl parity)")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/fleet/toggle", nil)
	req.Header.Set("Origin", "http://127.0.0.1:4173")
	if !sameOriginOK(req, cfg) {
		t.Error("expected matching Origin to pass")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/fleet/toggle", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	if sameOriginOK(req, cfg) {
		t.Error("expected cross-origin Origin to fail")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/fleet/toggle", nil)
	req.Header.Set("Referer", "http://evil.example.com/attack.html")
	if sameOriginOK(req, cfg) {
		t.Error("expected cross-origin Referer fallback to fail when Origin is absent")
	}
}

func TestGuardFleetToggle(t *testing.T) {
	loopbackCfg := fleetSecurityConfig{bindHost: "127.0.0.1", bindPort: 4173, loopbackBind: true}
	nonLoopbackCfg := fleetSecurityConfig{bindHost: "0.0.0.0", bindPort: 4173, fleetToken: "secret-token", loopbackBind: false}

	newJSONReq := func(host string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/fleet/toggle", nil)
		req.Header.Set("Content-Type", "application/json")
		if host != "" {
			req.Host = host
		}
		return req
	}

	// AC-3: missing/incorrect Content-Type is rejected.
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/toggle", nil)
	req.Host = "127.0.0.1:4173"
	if ok, _, _ := guardFleetToggle(req, loopbackCfg); ok {
		t.Error("expected missing Content-Type to be rejected")
	}
	req.Header.Set("Content-Type", "text/plain")
	if ok, _, _ := guardFleetToggle(req, loopbackCfg); ok {
		t.Error("expected non-JSON Content-Type to be rejected")
	}

	// AC-1: cross-origin (including the text/plain simple-request trick,
	// which is independently killed by the Content-Type check above).
	crossOrigin := newJSONReq("127.0.0.1:4173")
	crossOrigin.Header.Set("Origin", "http://evil.example.com")
	if ok, status, _ := guardFleetToggle(crossOrigin, loopbackCfg); ok || status != http.StatusForbidden {
		t.Errorf("expected cross-origin POST rejected 403, got ok=%v status=%v", ok, status)
	}

	// AC-2: Host header outside the allowed set is rejected before any
	// toggle logic would run.
	badHost := newJSONReq("evil.example.com:4173")
	if ok, status, _ := guardFleetToggle(badHost, loopbackCfg); ok || status != http.StatusForbidden {
		t.Errorf("expected disallowed Host rejected 403, got ok=%v status=%v", ok, status)
	}

	// Same-origin + matching Host + JSON content-type + loopback bind: no
	// token required, request passes.
	goodLoopback := newJSONReq("127.0.0.1:4173")
	if ok, _, msg := guardFleetToggle(goodLoopback, loopbackCfg); !ok {
		t.Errorf("expected loopback request without a token to pass, got msg=%q", msg)
	}

	// AC-4: non-loopback bind without the token header is rejected.
	noToken := newJSONReq("192.168.1.5:4173")
	if ok, status, _ := guardFleetToggle(noToken, nonLoopbackCfg); ok || status != http.StatusUnauthorized {
		t.Errorf("expected missing token rejected 401 on non-loopback bind, got ok=%v status=%v", ok, status)
	}

	// AC-4: non-loopback bind with the correct token passes.
	withToken := newJSONReq("192.168.1.5:4173")
	withToken.Header.Set("X-Quorum-Fleet-Token", "secret-token")
	if ok, _, msg := guardFleetToggle(withToken, nonLoopbackCfg); !ok {
		t.Errorf("expected correct token to pass on non-loopback bind, got msg=%q", msg)
	}

	// Wrong token still rejected.
	wrongToken := newJSONReq("192.168.1.5:4173")
	wrongToken.Header.Set("X-Quorum-Fleet-Token", "not-the-token")
	if ok, status, _ := guardFleetToggle(wrongToken, nonLoopbackCfg); ok || status != http.StatusUnauthorized {
		t.Errorf("expected wrong token rejected 401, got ok=%v status=%v", ok, status)
	}
}

func TestNewFleetToken(t *testing.T) {
	a, err := newFleetToken()
	if err != nil {
		t.Fatalf("newFleetToken: %v", err)
	}
	b, err := newFleetToken()
	if err != nil {
		t.Fatalf("newFleetToken: %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("expected non-empty tokens")
	}
	if a == b {
		t.Fatal("expected two independently generated tokens to differ")
	}
	if len(a) != 64 { // 32 bytes hex-encoded
		t.Fatalf("expected a 64-char hex token, got len=%d", len(a))
	}
}
