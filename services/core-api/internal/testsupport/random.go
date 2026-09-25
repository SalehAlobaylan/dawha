package testsupport

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"testing"
)

// randomToken returns a short lowercase hex token. Every fixture name and
// unique value is built from one, so two concurrent runs - or two runs of the
// same test with -count=2 - can never collide on a unique constraint.
func randomToken(t *testing.T) string {
	t.Helper()
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return hex.EncodeToString(buffer)
}

func filepathGlob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}
