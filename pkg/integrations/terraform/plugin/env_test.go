package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPluginEnv(t *testing.T) {
	execDir := t.TempDir()

	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SSL_CERT_FILE", "/cert.pem")
	t.Setenv("SSL_CERT_DIR", "/certs")
	t.Setenv("ENCRYPTION_KEY", "secret")
	t.Setenv("DATABASE_URL", "postgres://secret")
	t.Setenv("RABBITMQ_URL", "amqp://secret")
	t.Setenv("SECRET_KEY_BASE", "secret")
	t.Setenv("TF_CLI_ARGS_init", "-plugin-dir=/tmp/evil")
	t.Setenv("HOME", "/attacker-home")
	t.Setenv("TMPDIR", "/attacker-tmp")

	env, err := BuildPluginEnv(execDir)
	if err != nil {
		t.Fatalf("BuildPluginEnv() error = %v", err)
	}
	envMap := envMap(env)

	if envMap["HOME"] != filepath.Join(execDir, "home") {
		t.Fatalf("HOME = %q, want generated under exec dir", envMap["HOME"])
	}
	if envMap["TMPDIR"] != filepath.Join(execDir, "tmp") {
		t.Fatalf("TMPDIR = %q, want generated under exec dir", envMap["TMPDIR"])
	}
	for _, path := range []string{envMap["HOME"], envMap["TMPDIR"]} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat generated dir %q: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", path)
		}
	}

	if envMap["PATH"] != "/usr/bin:/bin" {
		t.Fatalf("PATH = %q", envMap["PATH"])
	}
	if envMap["SSL_CERT_FILE"] != "/cert.pem" {
		t.Fatalf("SSL_CERT_FILE = %q", envMap["SSL_CERT_FILE"])
	}
	if envMap["SSL_CERT_DIR"] != "/certs" {
		t.Fatalf("SSL_CERT_DIR = %q", envMap["SSL_CERT_DIR"])
	}
	for _, key := range []string{"ENCRYPTION_KEY", "DATABASE_URL", "RABBITMQ_URL", "SECRET_KEY_BASE", "TF_CLI_ARGS_init"} {
		if _, ok := envMap[key]; ok {
			t.Fatalf("%s leaked into plugin env", key)
		}
	}
}

func TestBuildPluginEnvRequiresExecDir(t *testing.T) {
	if _, err := BuildPluginEnv(""); err == nil {
		t.Fatal("expected error")
	}
}

func envMap(env []string) map[string]string {
	result := map[string]string{}
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}
