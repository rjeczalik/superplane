package plugin

import (
	"fmt"
	"os"
	"path/filepath"
)

func BuildPluginEnv(execDir string) ([]string, error) {
	if execDir == "" {
		return nil, fmt.Errorf("exec dir is required")
	}
	home := filepath.Join(execDir, "home")
	tmpdir := filepath.Join(execDir, "tmp")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("create plugin HOME: %w", err)
	}
	if err := os.MkdirAll(tmpdir, 0o700); err != nil {
		return nil, fmt.Errorf("create plugin TMPDIR: %w", err)
	}

	env := []string{
		"HOME=" + home,
		"TMPDIR=" + tmpdir,
	}
	for _, key := range []string{"PATH", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env, nil
}
