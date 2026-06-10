package indexer

import (
	"crypto/sha256"
	"fmt"

	"golang.org/x/text/unicode/norm"
)

// ComputeHash computes the SHA-256 hash of the content after NFC normalization.
// Returns the hash as a lowercase hex string.
func ComputeHash(content string) string {
	normalized := norm.NFC.String(content)
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", hash)
}
