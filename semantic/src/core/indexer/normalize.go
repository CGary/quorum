package indexer

import "strings"

// CanonicalizeName aplica la pipeline de §6.5 del Technical Specification:
// trim externo, collapse de whitespace interno, lowercase. El display
// preserva mayúsculas para legibilidad; el canonical es lo que deduplica
// (ej. "Redis", "redis", "REDIS" convergen todos en "redis").
func CanonicalizeName(raw string) (canonical, display string) {
	display = strings.TrimSpace(raw)
	canonical = strings.ToLower(strings.Join(strings.Fields(display), " "))
	return
}

// CanonicalizeType normaliza el enum de tipos (UPPERCASE, trimmed).
func CanonicalizeType(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}
