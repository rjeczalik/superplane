package terraform

import (
	"context"
	"encoding/json"

	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

type RuntimeValidator struct {
	runtimeFactory ConfiguredRuntimeFactory
}

func NewRuntimeValidator(factory ConfiguredRuntimeFactory) *RuntimeValidator {
	return &RuntimeValidator{runtimeFactory: factory}
}

func (v *RuntimeValidator) Validate(ctx context.Context, providerName, providerSource, providerVersion string, providerConfig map[string]any) error {
	rt, err := v.runtimeFactory.RuntimeForProvider(ctx, config.TerraformProviderIntegration{
		Name:    providerName,
		Source:  providerSource,
		Version: providerVersion,
	})
	if err != nil {
		return err
	}
	defer rt.Close()

	raw, err := json.Marshal(providerConfig)
	if err != nil {
		return err
	}
	prepared, err := rt.PrepareProviderConfig(ctx, &runtime.PrepareProviderConfigRequest{Config: runtime.DynamicValue{JSON: raw}})
	if err != nil {
		return err
	}
	configValue := runtime.DynamicValue{JSON: raw}
	if prepared != nil && len(prepared.PreparedConfig.JSON) > 0 {
		configValue = prepared.PreparedConfig
	}
	if err := rt.ValidateProviderConfig(ctx, &runtime.ValidateProviderConfigRequest{Config: configValue}); err != nil {
		return err
	}
	return rt.Configure(ctx, &runtime.ConfigureRequest{Config: configValue})
}
