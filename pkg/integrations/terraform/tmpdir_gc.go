package terraform

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	ExecTmpdirGCInterval = 15 * time.Minute
	ExecTmpdirGCMaxAge   = time.Hour
)

func GCExecTmpdirs(cacheDir string, maxAge time.Duration, logger *log.Entry) error {
	matches, err := filepath.Glob(filepath.Join(cacheDir, "exec-*"))
	if err != nil {
		return nil
	}

	cutoff := time.Now().Add(-maxAge)
	for _, match := range matches {
		info, err := os.Stat(match)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			if logger != nil {
				logger.WithError(err).WithField("path", match).Warn("failed to stat terraform exec tmpdir")
			}
			continue
		}
		if !info.IsDir() || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(match); err != nil && logger != nil {
			logger.WithError(err).WithField("path", match).Warn("failed to remove terraform exec tmpdir")
		}
	}
	return nil
}

func StartPeriodicGC(ctx context.Context, cacheDir string, interval time.Duration, maxAge time.Duration, logger *log.Entry) func() {
	if interval <= 0 {
		interval = ExecTmpdirGCInterval
	}
	if maxAge <= 0 {
		maxAge = ExecTmpdirGCMaxAge
	}

	gcCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var stopOnce sync.Once
	var finalGCOnce sync.Once
	finalGC := func() {
		finalGCOnce.Do(func() {
			_ = GCExecTmpdirs(cacheDir, maxAge, logger)
		})
	}

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = GCExecTmpdirs(cacheDir, maxAge, logger)
			case <-gcCtx.Done():
				finalGC()
				return
			}
		}
	}()

	return func() {
		stopOnce.Do(func() {
			cancel()
			<-done
			finalGC()
		})
	}
}
