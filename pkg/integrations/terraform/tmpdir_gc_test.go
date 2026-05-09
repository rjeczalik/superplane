package terraform

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCExecTmpdirs(t *testing.T) {
	dir := t.TempDir()
	oldDir := filepath.Join(dir, "exec-old")
	newDir := filepath.Join(dir, "exec-new")
	keepDir := filepath.Join(dir, "_schema", "talos")
	require.NoError(t, os.MkdirAll(oldDir, 0o700))
	require.NoError(t, os.MkdirAll(newDir, 0o700))
	require.NoError(t, os.MkdirAll(keepDir, 0o700))
	oldTime := time.Now().Add(-2 * ExecTmpdirGCMaxAge)
	require.NoError(t, os.Chtimes(oldDir, oldTime, oldTime))

	err := GCExecTmpdirs(dir, ExecTmpdirGCMaxAge, log.NewEntry(log.New()))
	require.NoError(t, err)

	_, err = os.Stat(oldDir)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(newDir)
	assert.NoError(t, err)
	_, err = os.Stat(keepDir)
	assert.NoError(t, err)
}

func TestGCExecTmpdirs_MissingCacheDirNoop(t *testing.T) {
	err := GCExecTmpdirs(filepath.Join(t.TempDir(), "missing"), ExecTmpdirGCMaxAge, log.NewEntry(log.New()))
	require.NoError(t, err)
}

func TestStartPeriodicGC(t *testing.T) {
	dir := t.TempDir()
	oldDir := filepath.Join(dir, "exec-old")
	require.NoError(t, os.MkdirAll(oldDir, 0o700))
	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(oldDir, oldTime, oldTime))

	stop := StartPeriodicGC(context.Background(), dir, 10*time.Millisecond, time.Hour, log.NewEntry(log.New()))
	t.Cleanup(stop)

	require.Eventually(t, func() bool {
		_, err := os.Stat(oldDir)
		return os.IsNotExist(err)
	}, time.Second, 10*time.Millisecond)
}

func TestStartPeriodicGCStopRunsFinalGC(t *testing.T) {
	dir := t.TempDir()
	oldDir := filepath.Join(dir, "exec-old")
	require.NoError(t, os.MkdirAll(oldDir, 0o700))
	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(oldDir, oldTime, oldTime))

	stop := StartPeriodicGC(context.Background(), dir, time.Hour, time.Hour, log.NewEntry(log.New()))
	stop()
	stop()

	_, err := os.Stat(oldDir)
	require.True(t, os.IsNotExist(err))
}

func TestStartPeriodicGCCanceledContextRunsFinalGC(t *testing.T) {
	dir := t.TempDir()
	oldDir := filepath.Join(dir, "exec-old")
	require.NoError(t, os.MkdirAll(oldDir, 0o700))
	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(oldDir, oldTime, oldTime))

	ctx, cancel := context.WithCancel(context.Background())
	stop := StartPeriodicGC(ctx, dir, time.Hour, time.Hour, log.NewEntry(log.New()))
	defer stop()
	cancel()

	require.Eventually(t, func() bool {
		_, err := os.Stat(oldDir)
		return os.IsNotExist(err)
	}, time.Second, 10*time.Millisecond)
}
