package terraform

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/crypto"
	tfregistry "github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
	"github.com/superplanehq/superplane/pkg/registry"
)

const startupTimeoutDefault = 5 * time.Minute

type RegisterOptions struct {
	Providers                   []config.TerraformProviderIntegration
	CacheDir                    string
	ManagedResourceStore        ManagedResourceStore
	Encryptor                   crypto.Encryptor
	ExecutionTimeout            time.Duration
	Logger                      *log.Entry
	RuntimeFactory              ConfiguredRuntimeFactory
	NewConfiguredRuntimeFactory func(cacheDir string) (ConfiguredRuntimeFactory, error)
	AppEnv                      string
	GPGVerifyMode               string
	GPGKeyStore                 *tfregistry.KeyPinStore
	PluginIsolationMode         string
}

func RegisterConfiguredProviders(ctx context.Context, opts RegisterOptions) error {
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, startupTimeoutDefault)
	defer cancel()

	cfgs := opts.Providers
	if cfgs == nil {
		parsed, err := config.TerraformProviderIntegrations()
		if err != nil {
			return err
		}
		cfgs = parsed
	}
	if len(cfgs) == 0 {
		return nil
	}

	for _, cfg := range cfgs {
		if exposeContainsWildcard(cfg.Expose.Resources) && !allowsWildcardResourceExposure(opts.AppEnv) {
			return fmt.Errorf("terraform provider %q: expose.resources wildcard is only allowed in development or test", cfg.Name)
		}
	}

	logger := opts.Logger
	if logger == nil {
		logger = log.WithField("component", "terraform-providers")
	}
	if err := GCExecTmpdirs(opts.CacheDir, ExecTmpdirGCMaxAge, logger); err != nil {
		return err
	}
	StartPeriodicGC(parentCtx, opts.CacheDir, ExecTmpdirGCInterval, ExecTmpdirGCMaxAge, logger)

	runtimeFactory := opts.RuntimeFactory
	if runtimeFactory == nil {
		newFactory := opts.NewConfiguredRuntimeFactory
		if newFactory == nil {
			newFactory = func(cacheDir string) (ConfiguredRuntimeFactory, error) {
				return NewProviderRuntimeFactory(ProviderRuntimeFactoryOptions{
					CacheDir:            cacheDir,
					AppEnv:              opts.AppEnv,
					GPGVerifyMode:       opts.GPGVerifyMode,
					GPGKeyStore:         opts.GPGKeyStore,
					PluginIsolationMode: opts.PluginIsolationMode,
				})
			}
		}
		var err error
		runtimeFactory, err = newFactory(opts.CacheDir)
		if err != nil {
			return fmt.Errorf("terraform direct runtime precondition failed: %w", err)
		}
	}

	var loader interface {
		Load(context.Context, config.TerraformProviderIntegration) (*ProviderSchemasFile, error)
	}
	var validator TerraformValidator
	loader = NewRuntimeSchemaLoader(runtimeFactory)
	validator = NewRuntimeValidator(runtimeFactory)

	executionTimeout := opts.ExecutionTimeout
	if executionTimeout == 0 {
		var err error
		executionTimeout, err = config.TerraformExecutionTimeoutDefaultE()
		if err != nil {
			return err
		}
	}

	for _, cfg := range cfgs {
		schemas, err := loader.Load(ctx, cfg)
		if err != nil {
			return fmt.Errorf("load %s: %w", cfg.Name, err)
		}
		var runner ActionRunner
		var resourceRunner *ResourceRunner
		if runtimeFactory != nil {
			providerCfg := cfg
			runner = NewDirectRunner(func(execCtx context.Context, spec *runtime.ExecutionSpec) (runtime.ProviderRuntime, error) {
				return runtimeFactory.RuntimeForProvider(execCtx, providerCfg)
			}, executionTimeout, WithDirectRunnerAudit(NewAuditLogger(logger)))
			if opts.ManagedResourceStore != nil {
				resourceRunner = NewResourceRunner(opts.ManagedResourceStore, runtimeFactory, executionTimeout)
			}
		}
		gen, dropped, err := BuildIntegration(cfg, *schemas, validator, runner, resourceRunner, logger)
		if err != nil {
			return fmt.Errorf("build %s: %w", cfg.Name, err)
		}
		for _, d := range dropped {
			logger.WithFields(log.Fields{"provider": cfg.Name, "capability": d.Name}).Warn(d.Reason)
		}

		registry.RegisterIntegrationWithOptions(gen.Name(), gen, registry.IntegrationRegistrationOptions{
			SetupProvider: gen.SetupProvider(),
		})
	}
	return nil
}

func allowsWildcardResourceExposure(appEnv string) bool {
	return appEnv == "" || appEnv == "development" || appEnv == "test"
}

func exposeContainsWildcard(value any) bool {
	switch v := value.(type) {
	case string:
		return v == "*"
	case []string:
		for _, item := range v {
			if item == "*" {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == "*" {
				return true
			}
		}
	}
	return false
}
