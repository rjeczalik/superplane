package testutil

import (
	"encoding/json"
	"os"
	"testing"
)

// SetEnvJSON JSON-encodes value and sets it as an environment variable for the
// duration of the test using t.Setenv.
func SetEnvJSON(t *testing.T, key string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal env value for %s: %v", key, err)
	}
	t.Setenv(key, string(b))
}

// GetEnv returns the current value of an environment variable. It is intended
// for test assertions after SetEnvJSON.
func GetEnv(t *testing.T, key string) string {
	t.Helper()
	return os.Getenv(key)
}
