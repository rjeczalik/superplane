package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// LoadFixture reads a file under pkg/integrations/terraform/testdata/<relPath>
// and returns its bytes. It fails the test if the file is missing or unreadable.
func LoadFixture(t *testing.T, relPath string) []byte {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	dir := filepath.Dir(file)
	fullPath := filepath.Join(dir, "..", "..", "testdata", relPath)

	b, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("load fixture %q: %v", relPath, err)
	}
	return b
}
