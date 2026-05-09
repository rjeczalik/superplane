package plugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/singleflight"
)

type BinaryCache struct {
	dir     string
	maxSize int64
	mu      sync.Mutex
	sf      singleflight.Group
}

type BinaryCacheKey struct {
	Source   string
	Version  string
	Platform string
	SHA256   string
}

func NewBinaryCache(dir string, maxSize int64) (*BinaryCache, error) {
	if dir == "" {
		return nil, fmt.Errorf("binary cache dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create binary cache dir: %w", err)
	}
	if err := validateCacheRoot(dir); err != nil {
		return nil, err
	}
	return &BinaryCache{dir: dir, maxSize: maxSize}, nil
}

func (c *BinaryCache) Get(key BinaryCacheKey) (string, bool, error) {
	if err := key.Validate(); err != nil {
		return "", false, err
	}
	path := c.path(key)
	info, err := safeCachedFileInfo(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if err := verifyFileChecksum(path, key.SHA256); err != nil {
		return "", false, err
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return path, info.Mode().IsRegular(), nil
}

func (c *BinaryCache) Store(key BinaryCacheKey, executable []byte) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := key.Validate(); err != nil {
		return "", err
	}
	if err := verifyBytesChecksum(executable, key.SHA256); err != nil {
		return "", err
	}
	target := c.path(key)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", fmt.Errorf("create binary cache shard: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), ".store-*")
	if err != nil {
		return "", fmt.Errorf("create binary cache temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(executable); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write binary cache temp file: %w", err)
	}
	if err := tmp.Chmod(0o500); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("chmod binary cache temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync binary cache temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close binary cache temp file: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", fmt.Errorf("publish binary cache entry: %w", err)
	}
	_ = fsyncDir(filepath.Dir(target))

	if c.maxSize > 0 {
		if err := c.evictLocked(c.maxSize); err != nil {
			return "", err
		}
	}
	return target, nil
}

func (c *BinaryCache) GetOrStore(key BinaryCacheKey, download func() ([]byte, error)) (string, error) {
	path, ok, err := c.Get(key)
	if err != nil {
		return "", err
	}
	if ok {
		return path, nil
	}
	v, err, _ := c.sf.Do(key.SHA256, func() (any, error) {
		path, ok, err := c.Get(key)
		if err != nil || ok {
			return path, err
		}
		raw, err := download()
		if err != nil {
			return "", err
		}
		return c.Store(key, raw)
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (c *BinaryCache) Evict(maxSize int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.evictLocked(maxSize)
}

func (c *BinaryCache) evictLocked(maxSize int64) error {
	entries, total, err := c.entries()
	if err != nil {
		return err
	}
	if total <= maxSize {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime.Before(entries[j].modTime)
	})
	for _, entry := range entries {
		if total <= maxSize {
			break
		}
		if err := os.Remove(entry.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict binary cache entry: %w", err)
		}
		total -= entry.size
	}
	return nil
}

func (c *BinaryCache) path(key BinaryCacheKey) string {
	return filepath.Join(c.dir, key.SHA256[:2], key.SHA256)
}

func (k BinaryCacheKey) Validate() error {
	if k.Source == "" {
		return fmt.Errorf("binary cache key source is required")
	}
	if k.Version == "" {
		return fmt.Errorf("binary cache key version is required")
	}
	if k.Platform == "" {
		return fmt.Errorf("binary cache key platform is required")
	}
	if _, err := hex.DecodeString(k.SHA256); err != nil || len(k.SHA256) != 64 {
		return fmt.Errorf("binary cache key sha256 must be a 64-character hex string")
	}
	return nil
}

type cacheEntry struct {
	path    string
	size    int64
	modTime time.Time
}

func (c *BinaryCache) entries() ([]cacheEntry, int64, error) {
	var entries []cacheEntry
	var total int64
	err := filepath.WalkDir(c.dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		info, err := safeCachedFileInfo(path)
		if err != nil {
			return err
		}
		total += info.Size()
		entries = append(entries, cacheEntry{path: path, size: info.Size(), modTime: info.ModTime()})
		return nil
	})
	return entries, total, err
}

func validateCacheRoot(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat binary cache dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("binary cache root is not a directory")
	}
	if info.Mode().Perm()&0o002 != 0 {
		return fmt.Errorf("binary cache root must not be world-writable")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("binary cache root must be owned by the current user")
	}
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("binary cache root contains symlink %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		_, err = safeCachedFileInfo(path)
		return err
	})
}

func safeCachedFileInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("binary cache entry %s is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("binary cache entry %s is not a regular file", path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink > 1 {
		return nil, fmt.Errorf("binary cache entry %s has hardlinks", path)
	}
	return info, nil
}

func verifyFileChecksum(path string, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open binary cache entry: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("read binary cache entry: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("binary cache checksum mismatch: got %s, want %s", actual, expected)
	}
	return nil
}

func VerifyFileChecksum(path string, expected string) error {
	return verifyFileChecksum(path, expected)
}

func BytesSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func verifyBytesChecksum(raw []byte, expected string) error {
	sum := sha256.Sum256(raw)
	actual := hex.EncodeToString(sum[:])
	if !bytes.Equal([]byte(actual), []byte(expected)) {
		return fmt.Errorf("binary checksum mismatch: got %s, want %s", actual, expected)
	}
	return nil
}
