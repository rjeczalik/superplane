package plugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	goplugin "github.com/hashicorp/go-plugin"
	tf5client "github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov5/tf5client"
	tf6client "github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov6/tf6client"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

const (
	terraformPluginName        = "provider"
	terraformMagicCookieKey    = "TF_PLUGIN_MAGIC_COOKIE"
	terraformMagicCookieValue  = "d602bf8f470bc67ca7faa0386276bbdd4330efaf76d1a219cb4d6991ca9872b2"
	defaultPluginStartTimeout  = 30 * time.Second
	defaultPluginExecDirPrefix = "provider-"
)

type PluginLauncher struct {
	cache        *BinaryCache
	limiter      *OrgRateLimiter
	execRoot     string
	startTimeout time.Duration
	audit        LaunchAuditLogger
	telemetry    LaunchTelemetry
}

type LauncherOptions struct {
	Cache        *BinaryCache
	Limiter      *OrgRateLimiter
	ExecRoot     string
	StartTimeout time.Duration
	Audit        LaunchAuditLogger
	Telemetry    LaunchTelemetry
}

type LaunchRequest struct {
	OrgID            string
	Key              BinaryCacheKey
	ExecutableSHA256 string
	ProtocolMajor    int
	Isolation        IsolationOptions
	ProviderName     string
	ProviderSource   string
	ProviderVersion  string
}

type LaunchAuditLogger interface {
	LogProviderLaunch(providerName, providerSource, providerVersion string, protocolMajor int)
}

type LaunchTelemetry interface {
	StartPluginLaunch(ctx context.Context, providerName, providerSource, providerVersion string, protocolMajor int) (context.Context, func(error))
	RecordCacheResult(ctx context.Context, providerName, providerSource, providerVersion string, hit bool)
	RecordVerificationFailure(ctx context.Context, providerName, providerSource, providerVersion string)
	AddQueueDepth(ctx context.Context, providerName, providerSource, providerVersion string, delta int64)
}

type LaunchedPlugin struct {
	Client        *goplugin.Client
	Server        any
	ProtocolMajor int
	release       func()
	execDir       string
}

func NewPluginLauncher(opts LauncherOptions) (*PluginLauncher, error) {
	if opts.Cache == nil {
		return nil, fmt.Errorf("binary cache is required")
	}
	limiter := opts.Limiter
	if limiter == nil {
		limiter = NewOrgRateLimiter(0, nil)
	}
	execRoot := opts.ExecRoot
	if execRoot == "" {
		execRoot = os.TempDir()
	}
	startTimeout := opts.StartTimeout
	if startTimeout == 0 {
		startTimeout = defaultPluginStartTimeout
	}
	return &PluginLauncher{
		cache:        opts.Cache,
		limiter:      limiter,
		execRoot:     execRoot,
		startTimeout: startTimeout,
		audit:        opts.Audit,
		telemetry:    opts.Telemetry,
	}, nil
}

func (l *PluginLauncher) Launch(ctx context.Context, req LaunchRequest) (*LaunchedPlugin, error) {
	ctx, finishLaunch := l.startLaunchTelemetry(ctx, req)
	var launchErr error
	defer func() {
		finishLaunch(launchErr)
	}()

	release, err := l.acquire(ctx, req)
	if err != nil {
		launchErr = err
		return nil, err
	}
	cleanupRelease := true
	defer func() {
		if cleanupRelease {
			release()
		}
	}()

	binaryPath, ok, err := l.cache.Get(req.Key)
	if err != nil {
		launchErr = err
		l.recordVerificationFailure(ctx, req)
		return nil, fmt.Errorf("verify provider binary before launch: %w", err)
	}
	l.recordCacheResult(ctx, req, ok)
	if !ok {
		launchErr = fmt.Errorf("provider binary is not cached")
		return nil, launchErr
	}

	plugin, err := l.launchProcess(ctx, binaryPath, req, release)
	if err != nil {
		launchErr = err
		return nil, err
	}
	cleanupRelease = false
	return plugin, nil
}

func (l *PluginLauncher) LaunchBinary(ctx context.Context, binaryPath string, req LaunchRequest) (*LaunchedPlugin, error) {
	ctx, finishLaunch := l.startLaunchTelemetry(ctx, req)
	var launchErr error
	defer func() {
		finishLaunch(launchErr)
	}()

	release, err := l.acquire(ctx, req)
	if err != nil {
		launchErr = err
		return nil, err
	}
	cleanupRelease := true
	defer func() {
		if cleanupRelease {
			release()
		}
	}()

	if req.ExecutableSHA256 != "" {
		if err := VerifyFileChecksum(binaryPath, req.ExecutableSHA256); err != nil {
			launchErr = err
			l.recordVerificationFailure(ctx, req)
			return nil, fmt.Errorf("verify provider binary before launch: %w", err)
		}
	}

	plugin, err := l.launchProcess(ctx, binaryPath, req, release)
	if err != nil {
		launchErr = err
		return nil, err
	}
	cleanupRelease = false
	return plugin, nil
}

func (l *PluginLauncher) acquire(ctx context.Context, req LaunchRequest) (func(), error) {
	l.addQueueDepth(ctx, req, 1)
	release, err := l.limiter.Acquire(ctx, req.OrgID)
	l.addQueueDepth(ctx, req, -1)
	return release, err
}

func (l *PluginLauncher) launchProcess(ctx context.Context, binaryPath string, req LaunchRequest, release func()) (*LaunchedPlugin, error) {
	execDir, err := os.MkdirTemp(l.execRoot, defaultPluginExecDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("create provider exec dir: %w", err)
	}
	cleanupExecDir := true
	defer func() {
		if cleanupExecDir {
			_ = os.RemoveAll(execDir)
		}
	}()

	config, err := l.clientConfig(ctx, binaryPath, execDir, req)
	if err != nil {
		return nil, err
	}
	if l.audit != nil {
		l.audit.LogProviderLaunch(req.ProviderName, req.ProviderSource, req.ProviderVersion, req.ProtocolMajor)
	}
	client := goplugin.NewClient(config)
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, &runtime.PluginLifecycleError{Phase: "handshake", Cause: err}
	}
	raw, err := rpcClient.Dispense(terraformPluginName)
	if err != nil {
		client.Kill()
		return nil, &runtime.PluginLifecycleError{Phase: "dispense", Cause: err}
	}
	if raw == nil {
		client.Kill()
		return nil, &runtime.PluginLifecycleError{Phase: "dispense", Cause: fmt.Errorf("provider client is nil")}
	}

	cleanupExecDir = false
	return &LaunchedPlugin{
		Client:        client,
		Server:        raw,
		ProtocolMajor: req.ProtocolMajor,
		release:       release,
		execDir:       execDir,
	}, nil
}

func (l *PluginLauncher) startLaunchTelemetry(ctx context.Context, req LaunchRequest) (context.Context, func(error)) {
	if l.telemetry == nil {
		return ctx, func(error) {}
	}
	return l.telemetry.StartPluginLaunch(ctx, req.ProviderName, req.ProviderSource, req.ProviderVersion, req.ProtocolMajor)
}

func (l *PluginLauncher) recordCacheResult(ctx context.Context, req LaunchRequest, hit bool) {
	if l.telemetry != nil {
		l.telemetry.RecordCacheResult(ctx, req.ProviderName, req.ProviderSource, req.ProviderVersion, hit)
	}
}

func (l *PluginLauncher) recordVerificationFailure(ctx context.Context, req LaunchRequest) {
	if l.telemetry != nil {
		l.telemetry.RecordVerificationFailure(ctx, req.ProviderName, req.ProviderSource, req.ProviderVersion)
	}
}

func (l *PluginLauncher) addQueueDepth(ctx context.Context, req LaunchRequest, delta int64) {
	if l.telemetry != nil {
		l.telemetry.AddQueueDepth(ctx, req.ProviderName, req.ProviderSource, req.ProviderVersion, delta)
	}
}

func (p *LaunchedPlugin) Close() {
	if p == nil {
		return
	}
	if p.Client != nil {
		p.Client.Kill()
	}
	if p.execDir != "" {
		_ = os.RemoveAll(p.execDir)
	}
	if p.release != nil {
		p.release()
	}
}

func (l *PluginLauncher) clientConfig(ctx context.Context, binaryPath string, execDir string, req LaunchRequest) (*goplugin.ClientConfig, error) {
	if req.ProtocolMajor != 5 && req.ProtocolMajor != 6 {
		return nil, fmt.Errorf("unsupported Terraform plugin protocol %d", req.ProtocolMajor)
	}
	env, err := BuildPluginEnv(execDir)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Dir = execDir
	cmd.Env = env
	if err := ApplyIsolation(cmd, req.Isolation); err != nil {
		return nil, err
	}

	return &goplugin.ClientConfig{
		Cmd:              cmd,
		HandshakeConfig:  TerraformHandshakeConfig(),
		VersionedPlugins: TerraformVersionedPluginsForProtocol(req.ProtocolMajor),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		AutoMTLS:         true,
		Managed:          false,
		StartTimeout:     l.startTimeout,
		SkipHostEnv:      true,
		Stderr:           new(bytes.Buffer),
		SyncStderr:       new(bytes.Buffer),
		SyncStdout:       bytes.NewBuffer(nil),
	}, nil
}

func TerraformHandshakeConfig() goplugin.HandshakeConfig {
	return goplugin.HandshakeConfig{
		MagicCookieKey:   terraformMagicCookieKey,
		MagicCookieValue: terraformMagicCookieValue,
	}
}

func TerraformVersionedPlugins() map[int]goplugin.PluginSet {
	return map[int]goplugin.PluginSet{
		5: {terraformPluginName: &tf5client.GRPCClientPlugin{}},
		6: {terraformPluginName: &tf6client.GRPCClientPlugin{}},
	}
}

func TerraformVersionedPluginsForProtocol(protocolMajor int) map[int]goplugin.PluginSet {
	plugins := TerraformVersionedPlugins()
	selected, ok := plugins[protocolMajor]
	if !ok {
		return nil
	}
	return map[int]goplugin.PluginSet{protocolMajor: selected}
}

func (l *PluginLauncher) execDirForTest(p *LaunchedPlugin) string {
	if p == nil {
		return ""
	}
	return filepath.Clean(p.execDir)
}
