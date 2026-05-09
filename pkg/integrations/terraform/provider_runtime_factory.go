package terraform

import (
	"context"
	"fmt"
	"strings"

	"github.com/superplanehq/superplane/pkg/config"
	tfplugin "github.com/superplanehq/superplane/pkg/integrations/terraform/plugin"
	tfregistry "github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

type ProviderPackageResolver interface {
	ResolveProvider(context.Context, tfregistry.ProviderSource, tfregistry.ProviderVersion) (*tfregistry.ProviderPackage, error)
}

type ProviderRuntimeCreator func(context.Context, RuntimeLaunchRequest) (runtime.ProviderRuntime, error)

type ProviderRuntimeFactoryOptions struct {
	Resolver            ProviderPackageResolver
	Launcher            *tfplugin.PluginLauncher
	CacheDir            string
	RuntimeCreator      ProviderRuntimeCreator
	AppEnv              string
	GPGVerifyMode       string
	GPGKeyStore         *tfregistry.KeyPinStore
	PluginIsolationMode string
}

type ProviderRuntimeFactory struct {
	resolver       ProviderPackageResolver
	cacheDir       string
	runtimeCreator ProviderRuntimeCreator
	isolation      tfplugin.IsolationOptions
}

func NewProviderRuntimeFactory(opts ProviderRuntimeFactoryOptions) (*ProviderRuntimeFactory, error) {
	if opts.CacheDir == "" {
		return nil, fmt.Errorf("terraform provider cache dir is required")
	}

	resolver := opts.Resolver
	if resolver == nil {
		registryOptions, err := registryResolverOptions(opts)
		if err != nil {
			return nil, err
		}
		resolver = tfregistry.NewRegistryResolver(registryOptions...)
	}

	runtimeCreator := opts.RuntimeCreator
	isolation, err := pluginIsolationOptions(opts)
	if err != nil {
		return nil, err
	}
	if runtimeCreator == nil {
		launcher := opts.Launcher
		if launcher == nil {
			cache, err := tfplugin.NewBinaryCache(opts.CacheDir, 0)
			if err != nil {
				return nil, err
			}
			launcher, err = tfplugin.NewPluginLauncher(tfplugin.LauncherOptions{Cache: cache})
			if err != nil {
				return nil, err
			}
		}
		runtimeCreator = NewRuntimeFactory(launcher).NewProviderRuntimeForLaunch
	}

	return &ProviderRuntimeFactory{
		resolver:       resolver,
		cacheDir:       opts.CacheDir,
		runtimeCreator: runtimeCreator,
		isolation:      isolation,
	}, nil
}

func registryResolverOptions(opts ProviderRuntimeFactoryOptions) ([]tfregistry.ResolverOption, error) {
	appEnv := strings.ToLower(strings.TrimSpace(opts.AppEnv))
	verifyMode := strings.ToLower(strings.TrimSpace(opts.GPGVerifyMode))
	if verifyMode == "skip" {
		if appEnv == "production" {
			return nil, fmt.Errorf("cannot skip Terraform provider GPG verification in production")
		}
		return []tfregistry.ResolverOption{tfregistry.WithSignatureVerificationSkipped()}, nil
	}

	if opts.GPGKeyStore == nil {
		return nil, nil
	}

	return []tfregistry.ResolverOption{
		tfregistry.WithKeyPinStore(opts.GPGKeyStore, tfregistry.KeyPinPolicy{
			Production: appEnv == "production",
			AllowTOFU:  appEnv != "production",
		}),
	}, nil
}

func pluginIsolationOptions(opts ProviderRuntimeFactoryOptions) (tfplugin.IsolationOptions, error) {
	appEnv := strings.ToLower(strings.TrimSpace(opts.AppEnv))
	mode := strings.ToLower(strings.TrimSpace(opts.PluginIsolationMode))
	switch mode {
	case "", "isolated":
		return tfplugin.IsolationOptions{}, nil
	case "disabled", "none", "unisolated":
		if appEnv == "production" {
			return tfplugin.IsolationOptions{}, fmt.Errorf("cannot disable Terraform provider process isolation in production")
		}
		return tfplugin.IsolationOptions{Egress: tfplugin.EgressUnisolated}, nil
	default:
		return tfplugin.IsolationOptions{}, fmt.Errorf("unknown Terraform provider process isolation mode %q", opts.PluginIsolationMode)
	}
}

func (f *ProviderRuntimeFactory) RuntimeForProvider(ctx context.Context, cfg config.TerraformProviderIntegration) (runtime.ProviderRuntime, error) {
	if f == nil {
		return nil, fmt.Errorf("terraform provider runtime factory is nil")
	}

	source, err := tfregistry.ParseProviderSource(cfg.Source)
	if err != nil {
		return nil, err
	}
	version, err := tfregistry.ParseProviderVersion(cfg.Version)
	if err != nil {
		return nil, err
	}

	pkg, err := f.resolver.ResolveProvider(ctx, source, version)
	if err != nil {
		return nil, err
	}
	if pkg == nil {
		return nil, fmt.Errorf("terraform registry resolver returned nil provider package")
	}

	extracted, err := tfplugin.ExtractProviderZipResult(pkg.Archive, tfplugin.ExtractOptions{
		CacheDir:     f.cacheDir,
		ProviderType: source.Type(),
		SHA256:       pkg.SHA256,
	})
	if err != nil {
		return nil, err
	}

	return f.runtimeCreator(ctx, RuntimeLaunchRequest{
		Protocols:        pkg.Protocols,
		BinaryPath:       extracted.Path,
		ExecutableSHA256: extracted.ExecutableSHA256,
		ProviderName:     cfg.Name,
		ProviderSource:   cfg.Source,
		ProviderVersion:  cfg.Version,
		Isolation:        f.isolation,
	})
}
