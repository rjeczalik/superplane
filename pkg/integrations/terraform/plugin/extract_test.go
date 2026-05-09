package plugin

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractProviderZip(t *testing.T) {
	archive := zipArchive(t, zipEntry{
		name: "terraform-provider-talos_0.11.0",
		mode: 0o755,
		body: "binary",
	})
	path, err := ExtractProviderZip(archive, ExtractOptions{
		CacheDir:     t.TempDir(),
		ProviderType: "talos",
		SHA256:       BytesSHA256(archive),
	})
	if err != nil {
		t.Fatalf("ExtractProviderZip() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat extracted executable: %v", err)
	}
	if info.Mode().Perm() != 0o500 {
		t.Fatalf("mode = %v, want 0500", info.Mode().Perm())
	}
	if filepath.Base(path) != BytesSHA256([]byte("binary")) {
		t.Fatalf("path = %q, want executable-addressed path", path)
	}
}

func TestExtractProviderZipRejectsArchiveChecksumMismatch(t *testing.T) {
	archive := zipArchive(t, zipEntry{name: "terraform-provider-talos", mode: 0o755, body: "binary"})
	_, err := ExtractProviderZip(archive, ExtractOptions{
		CacheDir:     t.TempDir(),
		ProviderType: "talos",
		SHA256:       strings.Repeat("b", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "archive checksum mismatch") {
		t.Fatalf("error = %v, want archive checksum mismatch", err)
	}
}

func TestExtractProviderZipRejectsUnsafeArchives(t *testing.T) {
	tests := []struct {
		name    string
		archive []byte
		opts    ExtractOptions
		want    string
	}{
		{
			name:    "path traversal",
			archive: zipArchive(t, zipEntry{name: "../terraform-provider-talos", mode: 0o755, body: "binary"}),
			want:    "unsafe path",
		},
		{
			name:    "absolute path",
			archive: zipArchive(t, zipEntry{name: "/terraform-provider-talos", mode: 0o755, body: "binary"}),
			want:    "unsafe path",
		},
		{
			name:    "symlink",
			archive: zipArchive(t, zipEntry{name: "terraform-provider-talos", mode: os.ModeSymlink | 0o777, body: "target"}),
			want:    "regular file",
		},
		{
			name:    "non regular file",
			archive: zipArchive(t, zipEntry{name: "terraform-provider-talos", mode: os.ModeDevice | 0o755, body: "binary"}),
			want:    "regular file",
		},
		{
			name:    "decompressed size limit",
			archive: zipArchive(t, zipEntry{name: "terraform-provider-talos", mode: 0o755, body: "binary"}),
			opts:    ExtractOptions{MaxBytes: 1},
			want:    "expands to more than",
		},
		{
			name: "file count limit",
			archive: zipArchive(t,
				zipEntry{name: "terraform-provider-talos", mode: 0o755, body: "binary"},
				zipEntry{name: "README.md", mode: 0o644, body: "readme"},
			),
			opts: ExtractOptions{MaxFiles: 1},
			want: "contains 2 files",
		},
		{
			name:    "zero executables",
			archive: zipArchive(t, zipEntry{name: "README.md", mode: 0o644, body: "readme"}),
			want:    "no executable",
		},
		{
			name: "multiple executables",
			archive: zipArchive(t,
				zipEntry{name: "terraform-provider-talos", mode: 0o755, body: "binary"},
				zipEntry{name: "terraform-provider-talos_helper", mode: 0o755, body: "binary"},
			),
			want: "multiple executables",
		},
		{
			name:    "wrong executable name",
			archive: zipArchive(t, zipEntry{name: "terraform-provider-cloudflare", mode: 0o755, body: "binary"}),
			want:    "does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			opts.CacheDir = t.TempDir()
			opts.ProviderType = "talos"
			opts.SHA256 = BytesSHA256(tt.archive)

			_, err := ExtractProviderZip(tt.archive, opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

type zipEntry struct {
	name string
	mode os.FileMode
	body string
}

func zipArchive(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()

	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		header.SetMode(entry.mode)
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("CreateHeader() error = %v", err)
		}
		if _, err := file.Write([]byte(entry.body)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return out.Bytes()
}
