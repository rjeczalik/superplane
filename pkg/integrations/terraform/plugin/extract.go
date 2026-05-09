package plugin

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMaxExtractedBytes = 1 << 30
	defaultMaxExtractedFiles = 64
)

type ExtractOptions struct {
	CacheDir     string
	ProviderType string
	SHA256       string
	MaxBytes     int64
	MaxFiles     int
}

func ExtractProviderZip(archive []byte, opts ExtractOptions) (string, error) {
	result, err := ExtractProviderZipResult(archive, opts)
	if err != nil {
		return "", err
	}
	return result.Path, nil
}

type ExtractResult struct {
	Path             string
	ArchiveSHA256    string
	ExecutableSHA256 string
}

func ExtractProviderZipResult(archive []byte, opts ExtractOptions) (*ExtractResult, error) {
	if opts.CacheDir == "" {
		return nil, fmt.Errorf("cache dir is required")
	}
	if opts.ProviderType == "" {
		return nil, fmt.Errorf("provider type is required")
	}
	if opts.SHA256 == "" || len(opts.SHA256) < 2 {
		return nil, fmt.Errorf("provider sha256 is required")
	}
	archiveSHA := BytesSHA256(archive)
	if archiveSHA != opts.SHA256 {
		return nil, fmt.Errorf("provider archive checksum mismatch: got %s, want %s", archiveSHA, opts.SHA256)
	}
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxExtractedBytes
	}
	maxFiles := opts.MaxFiles
	if maxFiles == 0 {
		maxFiles = defaultMaxExtractedFiles
	}

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open provider zip: %w", err)
	}
	if len(reader.File) > maxFiles {
		return nil, fmt.Errorf("provider zip contains %d files, max %d", len(reader.File), maxFiles)
	}

	if err := os.MkdirAll(opts.CacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	tmpdir, err := os.MkdirTemp(opts.CacheDir, ".extract-*")
	if err != nil {
		return nil, fmt.Errorf("create extraction temp dir: %w", err)
	}
	defer os.RemoveAll(tmpdir)

	var executable string
	var total int64
	for _, file := range reader.File {
		rel, err := safeZipPath(file.Name)
		if err != nil {
			return nil, err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(tmpdir, rel), 0o700); err != nil {
				return nil, fmt.Errorf("create extracted directory: %w", err)
			}
			continue
		}
		mode := file.Mode()
		if mode.Type() != 0 {
			return nil, fmt.Errorf("provider zip entry %q must be a regular file", file.Name)
		}
		total += int64(file.UncompressedSize64)
		if total > maxBytes {
			return nil, fmt.Errorf("provider zip expands to more than %d bytes", maxBytes)
		}

		target := filepath.Join(tmpdir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return nil, fmt.Errorf("create extracted parent: %w", err)
		}
		if err := extractFile(file, target); err != nil {
			return nil, err
		}
		if mode&0o111 == 0 {
			continue
		}
		if !validProviderExecutableName(filepath.Base(rel), opts.ProviderType) {
			return nil, fmt.Errorf("provider executable %q does not match terraform-provider-%s", filepath.Base(rel), opts.ProviderType)
		}
		if executable != "" {
			return nil, fmt.Errorf("provider zip contains multiple executables")
		}
		executable = target
	}
	if executable == "" {
		return nil, fmt.Errorf("provider zip contains no executable")
	}
	if err := os.Chmod(executable, 0o500); err != nil {
		return nil, fmt.Errorf("chmod provider executable: %w", err)
	}

	executableSHA, err := fileSHA256(executable)
	if err != nil {
		return nil, err
	}
	finalPath := filepath.Join(opts.CacheDir, executableSHA[:2], executableSHA)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o700); err != nil {
		return nil, fmt.Errorf("create cache shard: %w", err)
	}
	if err := os.Rename(executable, finalPath); err != nil {
		return nil, fmt.Errorf("publish provider executable: %w", err)
	}
	_ = fsyncDir(filepath.Dir(finalPath))
	_ = fsyncDir(opts.CacheDir)

	return &ExtractResult{Path: finalPath, ArchiveSHA256: archiveSHA, ExecutableSHA256: executableSHA}, nil
}

func safeZipPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("provider zip contains empty path")
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return "", fmt.Errorf("provider zip contains unsafe path %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("provider zip contains unsafe path %q", name)
	}
	return clean, nil
}

func extractFile(file *zip.File, target string) error {
	source, err := file.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %q: %w", file.Name, err)
	}
	defer source.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create extracted file %q: %w", file.Name, err)
	}
	_, copyErr := io.Copy(out, source)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("extract file %q: %w", file.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close extracted file %q: %w", file.Name, closeErr)
	}
	return nil
}

func validProviderExecutableName(name string, providerType string) bool {
	prefix := "terraform-provider-" + providerType
	return name == prefix || strings.HasPrefix(name, prefix+"_")
}

func fsyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open provider executable: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash provider executable: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
