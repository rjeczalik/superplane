package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBinaryCacheStoreAndGet(t *testing.T) {
	cache := newTestBinaryCache(t, 0)
	binary := []byte("provider binary")
	key := testCacheKey(binary)

	path, err := cache.Store(key, binary)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if filepath.Base(path) != key.SHA256 {
		t.Fatalf("path = %q, want content-addressed filename", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cache entry: %v", err)
	}
	if info.Mode().Perm() != 0o500 {
		t.Fatalf("mode = %v, want 0500", info.Mode().Perm())
	}

	got, ok, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() cache miss, want hit")
	}
	if got != path {
		t.Fatalf("Get() path = %q, want %q", got, path)
	}
}

func TestBinaryCacheVerifiesChecksumOnGet(t *testing.T) {
	cache := newTestBinaryCache(t, 0)
	key := testCacheKey([]byte("provider binary"))
	path, err := cache.Store(key, []byte("provider binary"))
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("make cache entry writable: %v", err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o500); err != nil {
		t.Fatalf("tamper cache entry: %v", err)
	}

	_, _, err = cache.Get(key)
	if err == nil {
		t.Fatal("expected checksum error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
}

func TestBinaryCacheMissTriggersDownload(t *testing.T) {
	cache := newTestBinaryCache(t, 0)
	binary := []byte("provider binary")
	key := testCacheKey(binary)
	var downloads int

	path, err := cache.GetOrStore(key, func() ([]byte, error) {
		downloads++
		return binary, nil
	})
	if err != nil {
		t.Fatalf("GetOrStore() error = %v", err)
	}
	if path == "" {
		t.Fatal("GetOrStore() returned empty path")
	}
	if downloads != 1 {
		t.Fatalf("downloads = %d, want 1", downloads)
	}

	if _, err := cache.GetOrStore(key, func() ([]byte, error) {
		downloads++
		return binary, nil
	}); err != nil {
		t.Fatalf("GetOrStore() cache hit error = %v", err)
	}
	if downloads != 1 {
		t.Fatalf("downloads after hit = %d, want 1", downloads)
	}
}

func TestBinaryCacheRejectsUnsafeRootAndEntries(t *testing.T) {
	t.Run("world writable root", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		_, err := NewBinaryCache(dir, 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("symlink entry", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Symlink("/bin/sh", filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
		_, err := NewBinaryCache(dir, 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestBinaryCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := newTestBinaryCache(t, int64(len("second")))
	first := []byte("first")
	second := []byte("second")
	firstKey := testCacheKey(first)
	secondKey := testCacheKey(second)

	firstPath, err := cache.Store(firstKey, first)
	if err != nil {
		t.Fatalf("Store(first) error = %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(firstPath, old, old); err != nil {
		t.Fatalf("age first cache entry: %v", err)
	}
	secondPath, err := cache.Store(secondKey, second)
	if err != nil {
		t.Fatalf("Store(second) error = %v", err)
	}

	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatalf("first path still exists, err=%v", err)
	}
	if _, err := os.Stat(secondPath); err != nil {
		t.Fatalf("second path missing: %v", err)
	}
}

func newTestBinaryCache(t *testing.T, maxSize int64) *BinaryCache {
	t.Helper()

	cache, err := NewBinaryCache(t.TempDir(), maxSize)
	if err != nil {
		t.Fatalf("NewBinaryCache() error = %v", err)
	}
	return cache
}

func testCacheKey(binary []byte) BinaryCacheKey {
	return BinaryCacheKey{
		Source:   "registry.terraform.io/siderolabs/talos",
		Version:  "0.11.0",
		Platform: "darwin_arm64",
		SHA256:   sha256Hex(binary),
	}
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
