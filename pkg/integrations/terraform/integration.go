package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

var _ core.Integration = (*GeneratedIntegration)(nil)
var _ core.OriginInfo = (*GeneratedIntegration)(nil)

// GeneratedIntegration is a core.Integration generated from a Terraform
// provider schema.
type GeneratedIntegration struct {
	name                 string
	label                string
	icon                 string
	description          string
	providerName         string
	providerSource       string
	providerVersion      string
	providerConfigSchema []configuration.Field
	actions              []core.Action
	triggers             []core.Trigger
	setupProvider        *GeneratedSetupProvider
	validator            TerraformValidator
}

func (i *GeneratedIntegration) Name() string {
	return i.name
}

func (i *GeneratedIntegration) Label() string {
	if i.label != "" {
		return i.label
	}
	return i.name
}

func (i *GeneratedIntegration) Icon() string {
	return i.icon
}

func (i *GeneratedIntegration) Description() string {
	if i.description != "" {
		return i.description
	}
	return fmt.Sprintf("Terraform provider %s", i.providerSource)
}

func (i *GeneratedIntegration) Instructions() string {
	return "Configure the Terraform provider, select capabilities, and SuperPlane will execute generated Terraform actions."
}

func (i *GeneratedIntegration) Configuration() []configuration.Field {
	return i.providerConfigSchema
}

func (i *GeneratedIntegration) Actions() []core.Action {
	return i.actions
}

func (i *GeneratedIntegration) Triggers() []core.Trigger {
	return i.triggers
}

func (i *GeneratedIntegration) Sync(ctx core.SyncContext) error {
	providerConfig, err := i.providerConfig(ctx)
	if err != nil {
		ctx.Integration.Error(err.Error())
		return err
	}

	if i.validator != nil {
		validateCtx, cancel := context.WithTimeout(context.Background(), terraformValidationTimeout)
		defer cancel()
		if err := i.validator.Validate(validateCtx, i.runtimeProviderName(), i.providerSource, i.providerVersion, providerConfig); err != nil {
			ctx.Integration.Error(err.Error())
			return err
		}
	}

	ctx.Integration.Ready()
	return nil
}

func (i *GeneratedIntegration) runtimeProviderName() string {
	if i.providerName != "" {
		return i.providerName
	}

	return i.name
}

func (i *GeneratedIntegration) Cleanup(ctx core.IntegrationCleanupContext) error {
	return nil
}

func (i *GeneratedIntegration) Hooks() []core.Hook {
	return nil
}

func (i *GeneratedIntegration) HandleHook(ctx core.IntegrationHookContext) error {
	return nil
}

func (i *GeneratedIntegration) ListResources(resourceType string, ctx core.ListResourcesContext) ([]core.IntegrationResource, error) {
	return nil, nil
}

func (i *GeneratedIntegration) HandleRequest(ctx core.HTTPRequestContext) {
	http.NotFound(ctx.Response, ctx.Request)
}

func (i *GeneratedIntegration) Origin() string {
	return "terraform"
}

func (i *GeneratedIntegration) Source() string {
	return i.providerSource
}

func (i *GeneratedIntegration) Version() string {
	return i.providerVersion
}

func (i *GeneratedIntegration) SetupProvider() *GeneratedSetupProvider {
	return i.setupProvider
}

func (i *GeneratedIntegration) providerConfig(ctx core.SyncContext) (map[string]any, error) {
	if ctx.Integration == nil || ctx.Integration.Secrets() == nil {
		return map[string]any{}, nil
	}

	raw, err := ctx.Integration.Secrets().Get(secretNameTerraformProviderConfig)
	if err != nil {
		if ctx.Logger != nil {
			ctx.Logger.WithError(err).Warn("terraform provider config secret missing; using empty config")
		} else {
			logrus.WithError(err).Warn("terraform provider config secret missing; using empty config")
		}
		return map[string]any{}, nil
	}

	if raw == "" {
		return map[string]any{}, nil
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, fmt.Errorf("decode terraform provider config: %w", err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}
