package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	goplugin "github.com/hashicorp/go-plugin"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
	"google.golang.org/grpc"
)

func TestPluginLauncherClientConfig(t *testing.T) {
	launcher := newTestLauncher(t, NewOrgRateLimiter(1, NewSemaphore(1)))
	execDir := t.TempDir()
	config, err := launcher.clientConfig(context.Background(), "/bin/echo", execDir, LaunchRequest{
		OrgID:         "org",
		Key:           testCacheKey([]byte("binary")),
		ProtocolMajor: 6,
	})
	if err != nil {
		t.Fatalf("clientConfig() error = %v", err)
	}

	if !config.AutoMTLS {
		t.Fatal("AutoMTLS is disabled")
	}
	if len(config.AllowedProtocols) != 1 || config.AllowedProtocols[0] != goplugin.ProtocolGRPC {
		t.Fatalf("AllowedProtocols = %#v, want grpc only", config.AllowedProtocols)
	}
	if config.HandshakeConfig.MagicCookieKey != terraformMagicCookieKey {
		t.Fatalf("MagicCookieKey = %q", config.HandshakeConfig.MagicCookieKey)
	}
	if config.HandshakeConfig.MagicCookieValue != terraformMagicCookieValue {
		t.Fatalf("MagicCookieValue = %q", config.HandshakeConfig.MagicCookieValue)
	}
	if _, ok := config.VersionedPlugins[5]; ok {
		t.Fatal("VersionedPlugins unexpectedly advertised v5 provider")
	}
	if _, ok := config.VersionedPlugins[6][terraformPluginName]; !ok {
		t.Fatal("VersionedPlugins missing v6 provider")
	}
	if config.Cmd.Env == nil {
		t.Fatal("plugin command env is empty")
	}
}

func TestPluginLauncherClientConfigRestrictsVersionedPluginsToRequestedProtocol(t *testing.T) {
	launcher := newTestLauncher(t, NewOrgRateLimiter(1, NewSemaphore(1)))
	execDir := t.TempDir()
	config, err := launcher.clientConfig(context.Background(), "/bin/echo", execDir, LaunchRequest{
		OrgID:         "org",
		Key:           testCacheKey([]byte("binary")),
		ProtocolMajor: 5,
	})
	if err != nil {
		t.Fatalf("clientConfig() error = %v", err)
	}

	if _, ok := config.VersionedPlugins[5][terraformPluginName]; !ok {
		t.Fatal("VersionedPlugins missing v5 provider")
	}
	if _, ok := config.VersionedPlugins[6]; ok {
		t.Fatal("VersionedPlugins unexpectedly advertised v6 provider")
	}
}

func TestPluginLauncherEmitsLaunchAuditWithoutSecrets(t *testing.T) {
	audit := &recordingLaunchAudit{}
	launcher := newTestLauncher(t, NewOrgRateLimiter(1, NewSemaphore(1)))
	launcher.audit = audit
	execDir := t.TempDir()
	_, err := launcher.clientConfig(context.Background(), "/bin/echo", execDir, LaunchRequest{
		OrgID:           "org",
		Key:             testCacheKey([]byte("binary")),
		ProtocolMajor:   6,
		ProviderName:    "cloudflare",
		ProviderSource:  "registry.terraform.io/cloudflare/cloudflare",
		ProviderVersion: "4.52.0",
	})
	if err != nil {
		t.Fatalf("clientConfig() error = %v", err)
	}
	launcher.audit.LogProviderLaunch("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", 6)

	if audit.providerName != "cloudflare" || audit.protocolMajor != 6 {
		t.Fatalf("audit = %#v, want provider launch", audit)
	}
	if strings.Contains(audit.serialized(), "secret-token") || strings.Contains(audit.serialized(), "TF_PLUGIN_MAGIC_COOKIE") {
		t.Fatalf("audit leaked sensitive value: %s", audit.serialized())
	}
}

func TestPluginLauncherBadBinaryFailsWithLifecycleErrorAndReleasesSemaphore(t *testing.T) {
	limiter := NewOrgRateLimiter(1, NewSemaphore(1))
	launcher := newTestLauncher(t, limiter)
	binary := []byte("#!/bin/sh\nexit 1\n")
	key := testCacheKey(binary)
	if _, err := launcher.cache.Store(key, binary); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	_, err := launcher.Launch(context.Background(), LaunchRequest{
		OrgID:         "org",
		Key:           key,
		ProtocolMajor: 6,
	})
	if err == nil {
		t.Fatal("expected launch error")
	}
	var lifecycle *runtime.PluginLifecycleError
	if !errors.As(err, &lifecycle) {
		t.Fatalf("error = %T %v, want PluginLifecycleError", err, err)
	}

	release, err := limiter.Acquire(context.Background(), "org")
	if err != nil {
		t.Fatalf("semaphore was not released after launch error: %v", err)
	}
	release()
}

type recordingLaunchAudit struct {
	providerName    string
	providerSource  string
	providerVersion string
	protocolMajor   int
}

func (a *recordingLaunchAudit) LogProviderLaunch(providerName, providerSource, providerVersion string, protocolMajor int) {
	a.providerName = providerName
	a.providerSource = providerSource
	a.providerVersion = providerVersion
	a.protocolMajor = protocolMajor
}

func (a *recordingLaunchAudit) serialized() string {
	return a.providerName + " " + a.providerSource + " " + a.providerVersion
}

func TestPluginLauncherVerifiesCachedBinaryBeforeLaunch(t *testing.T) {
	launcher := newTestLauncher(t, NewOrgRateLimiter(1, NewSemaphore(1)))
	binary := []byte("#!/bin/sh\nexit 1\n")
	key := testCacheKey(binary)
	path, err := launcher.cache.Store(key, binary)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("chmod cached binary: %v", err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o500); err != nil {
		t.Fatalf("tamper cached binary: %v", err)
	}

	_, err = launcher.Launch(context.Background(), LaunchRequest{
		OrgID:         "org",
		Key:           key,
		ProtocolMajor: 6,
	})
	if err == nil {
		t.Fatal("expected checksum error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
}

func TestPluginLauncherVerifiesExplicitBinaryBeforeLaunch(t *testing.T) {
	launcher := newTestLauncher(t, NewOrgRateLimiter(1, NewSemaphore(1)))
	path := filepath.Join(t.TempDir(), "terraform-provider-test")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o500); err != nil {
		t.Fatalf("write provider binary: %v", err)
	}

	_, err := launcher.LaunchBinary(context.Background(), path, LaunchRequest{
		OrgID:            "org",
		ExecutableSHA256: strings.Repeat("b", 64),
		ProtocolMajor:    6,
	})
	if err == nil {
		t.Fatal("expected checksum error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
}

func TestPluginLauncherRejectsUnsupportedProtocol(t *testing.T) {
	launcher := newTestLauncher(t, NewOrgRateLimiter(1, NewSemaphore(1)))
	_, err := launcher.clientConfig(context.Background(), "/bin/echo", t.TempDir(), LaunchRequest{ProtocolMajor: 4})
	if err == nil {
		t.Fatal("expected unsupported protocol error")
	}
}

func TestTerraformVersionedPluginsReturnProtocolClients(t *testing.T) {
	plugins := TerraformVersionedPlugins()

	v5Raw, err := plugins[5][terraformPluginName].(goplugin.GRPCPlugin).GRPCClient(context.Background(), nil, (*grpc.ClientConn)(nil))
	if err != nil {
		t.Fatalf("v5 GRPCClient() error = %v", err)
	}
	if _, ok := v5Raw.(tfprotov5.ProviderServer); !ok {
		t.Fatalf("v5 GRPCClient() = %T, want tfprotov5.ProviderServer", v5Raw)
	}

	v6Raw, err := plugins[6][terraformPluginName].(goplugin.GRPCPlugin).GRPCClient(context.Background(), nil, (*grpc.ClientConn)(nil))
	if err != nil {
		t.Fatalf("v6 GRPCClient() error = %v", err)
	}
	if _, ok := v6Raw.(tfprotov6.ProviderServer); !ok {
		t.Fatalf("v6 GRPCClient() = %T, want tfprotov6.ProviderServer", v6Raw)
	}
}

func newTestLauncher(t *testing.T, limiter *OrgRateLimiter) *PluginLauncher {
	t.Helper()

	cache, err := NewBinaryCache(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("NewBinaryCache() error = %v", err)
	}
	launcher, err := NewPluginLauncher(LauncherOptions{
		Cache:        cache,
		Limiter:      limiter,
		ExecRoot:     t.TempDir(),
		StartTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewPluginLauncher() error = %v", err)
	}
	return launcher
}
