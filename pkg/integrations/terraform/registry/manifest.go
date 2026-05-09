package registry

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

func ChecksumForFilename(sums []byte, filename string) (string, error) {
	if !safeManifestFilename(filename) {
		return "", fmt.Errorf("manifest filename %q is not safe", filename)
	}

	var found string
	scanner := bufio.NewScanner(strings.NewReader(string(sums)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		checksum, entryFilename, ok := parseManifestLine(line)
		if !ok {
			return "", fmt.Errorf("invalid SHA256SUMS line %q", line)
		}
		if !safeManifestFilename(entryFilename) {
			return "", fmt.Errorf("manifest entry filename %q is not safe", entryFilename)
		}
		if entryFilename != filename {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("manifest contains duplicate checksum entries for %q", filename)
		}
		found = checksum
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan SHA256SUMS manifest: %w", err)
	}
	if found == "" {
		return "", fmt.Errorf("manifest does not contain checksum for %q", filename)
	}

	return found, nil
}

func VerifyPackageChecksumBinding(sums []byte, filename string, jsonShasum string) (string, error) {
	manifestChecksum, err := ChecksumForFilename(sums, filename)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(manifestChecksum, jsonShasum) {
		return "", fmt.Errorf("registry shasum does not match signed manifest checksum for %q", filename)
	}

	return manifestChecksum, nil
}

func parseManifestLine(line string) (string, string, bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return "", "", false
	}
	if _, err := hex.DecodeString(fields[0]); err != nil || len(fields[0]) != 64 {
		return "", "", false
	}

	return strings.ToLower(fields[0]), strings.TrimPrefix(fields[1], "*"), true
}

func safeManifestFilename(filename string) bool {
	if filename == "" || filename != filepath.Base(filename) {
		return false
	}
	if strings.Contains(filename, `\`) || strings.Contains(filename, "..") {
		return false
	}

	return true
}
