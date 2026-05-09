package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const (
	setupStepCapabilitySelection = "capabilitySelection"
	setupStepProviderConfig      = "providerConfig"
	setupStepDone                = "done"

	secretNameTerraformProviderConfig = "terraformProviderConfig"
	terraformValidationTimeout        = 5 * time.Minute
)

// TerraformValidator validates provider configuration by asking Terraform to
// initialize and validate a minimal workspace.
type TerraformValidator interface {
	Validate(ctx context.Context, providerName, providerSource, providerVersion string, providerConfig map[string]any) error
}

// GeneratedSetupProvider implements the setup flow for a generated Terraform
// provider integration.
type GeneratedSetupProvider struct {
	integrationName      string
	providerName         string
	capabilityGroups     []core.CapabilityGroup
	providerConfigSchema []configuration.Field
	providerSource       string
	providerVersion      string
	validator            TerraformValidator
}

func (p *GeneratedSetupProvider) SupportsSetupFlowInProduction() bool {
	return true
}

func (p *GeneratedSetupProvider) CapabilityGroups() []core.CapabilityGroup {
	return p.capabilityGroups
}

func (p *GeneratedSetupProvider) FirstStep(ctx core.SetupStepContext) core.SetupStep {
	return core.SetupStep{
		Type:         core.SetupStepTypeCapabilitySelection,
		Name:         setupStepCapabilitySelection,
		Label:        "Select capabilities",
		Capabilities: p.allCapabilities(),
	}
}

func (p *GeneratedSetupProvider) OnStepSubmit(ctx core.SetupStepContext) (*core.SetupStep, error) {
	switch ctx.Step.Name {
	case setupStepCapabilitySelection:
		return p.submitCapabilitySelection(ctx)
	case setupStepProviderConfig:
		return p.submitProviderConfig(ctx)
	default:
		return nil, fmt.Errorf("unknown step %q", ctx.Step.Name)
	}
}

func (p *GeneratedSetupProvider) OnStepRevert(ctx core.SetupStepContext) error {
	switch ctx.Step.Name {
	case setupStepCapabilitySelection:
		if ctx.Capabilities != nil {
			ctx.Capabilities.Clear()
		}
		return nil
	case setupStepProviderConfig:
		if ctx.Secrets != nil {
			return ctx.Secrets.Delete(secretNameTerraformProviderConfig)
		}
		return nil
	default:
		return fmt.Errorf("unknown step %q", ctx.Step.Name)
	}
}

func (p *GeneratedSetupProvider) OnPropertyUpdate(ctx core.PropertyUpdateContext) (*core.SetupStep, error) {
	return nil, fmt.Errorf("property updates are unsupported in MVP")
}

func (p *GeneratedSetupProvider) OnSecretUpdate(ctx core.SecretUpdateContext) (*core.SetupStep, error) {
	if ctx.SecretName != secretNameTerraformProviderConfig {
		return nil, fmt.Errorf("unknown secret %q", ctx.SecretName)
	}

	return &core.SetupStep{
		Type:   core.SetupStepTypeInputs,
		Name:   setupStepProviderConfig,
		Label:  "Provider configuration",
		Inputs: p.providerConfigSchema,
	}, nil
}

func (p *GeneratedSetupProvider) OnCapabilityUpdate(ctx core.CapabilityUpdateContext) (*core.SetupStep, error) {
	valid := p.capabilitySet()
	for _, capabilities := range ctx.Changes {
		for _, capability := range capabilities {
			if _, ok := valid[capability]; !ok {
				return nil, fmt.Errorf("invalid capability %q", capability)
			}
		}
	}

	if requested := ctx.Changes[core.IntegrationCapabilityStateRequested]; len(requested) > 0 && ctx.Capabilities != nil {
		ctx.Capabilities.Enable(requested...)
	}

	return &core.SetupStep{Type: core.SetupStepTypeDone, Name: setupStepDone}, nil
}

func (p *GeneratedSetupProvider) submitCapabilitySelection(ctx core.SetupStepContext) (*core.SetupStep, error) {
	if err := p.validateCapabilities(ctx.Step.Capabilities); err != nil {
		return nil, err
	}

	if ctx.Capabilities != nil {
		selected := map[string]struct{}{}
		for _, capability := range ctx.Step.Capabilities {
			selected[capability] = struct{}{}
		}

		var available []string
		for _, capability := range p.allCapabilities() {
			if _, ok := selected[capability]; ok {
				ctx.Capabilities.Request(capability)
			} else {
				available = append(available, capability)
			}
		}
		if len(available) > 0 {
			ctx.Capabilities.Available(available...)
		}
	}

	if len(p.providerConfigSchema) == 0 {
		if err := p.persistProviderConfigAndEnable(ctx, map[string]any{}); err != nil {
			return nil, err
		}
		return &core.SetupStep{Type: core.SetupStepTypeDone, Name: setupStepDone}, nil
	}

	return &core.SetupStep{
		Type:   core.SetupStepTypeInputs,
		Name:   setupStepProviderConfig,
		Label:  "Provider configuration",
		Inputs: p.providerConfigSchema,
	}, nil
}

func (p *GeneratedSetupProvider) submitProviderConfig(ctx core.SetupStepContext) (*core.SetupStep, error) {
	inputs, ok := ctx.Step.Inputs.(map[string]any)
	if !ok && ctx.Step.Inputs != nil {
		return nil, fmt.Errorf("provider config inputs must be an object")
	}
	if inputs == nil {
		inputs = map[string]any{}
	}

	if err := configuration.ValidateConfiguration(p.providerConfigSchema, inputs); err != nil {
		return nil, err
	}

	if err := p.persistProviderConfigAndEnable(ctx, inputs); err != nil {
		return nil, err
	}

	return &core.SetupStep{Type: core.SetupStepTypeDone, Name: setupStepDone}, nil
}

func (p *GeneratedSetupProvider) persistProviderConfigAndEnable(ctx core.SetupStepContext, providerConfig map[string]any) error {
	if p.validator != nil {
		validateCtx, cancel := context.WithTimeout(context.Background(), terraformValidationTimeout)
		defer cancel()
		if err := p.validator.Validate(validateCtx, p.runtimeProviderName(), p.providerSource, p.providerVersion, providerConfig); err != nil {
			return err
		}
	}

	raw, err := json.Marshal(providerConfig)
	if err != nil {
		return err
	}

	if ctx.Secrets != nil {
		def := core.IntegrationSecretDefinition{
			Name:        secretNameTerraformProviderConfig,
			Label:       "Terraform provider config",
			Description: "Terraform provider configuration.",
			Value:       string(raw),
			Editable:    true,
		}
		if err := ctx.Secrets.Create(def); err != nil {
			if updateErr := ctx.Secrets.Update(secretNameTerraformProviderConfig, string(raw)); updateErr != nil {
				return err
			}
		}
	}

	if ctx.Capabilities != nil {
		requested := ctx.Capabilities.Requested()
		if len(requested) == 0 {
			requested = ctx.Step.Capabilities
		}
		if len(requested) > 0 {
			ctx.Capabilities.Enable(requested...)
		}
	}

	return nil
}

func (p *GeneratedSetupProvider) runtimeProviderName() string {
	if p.providerName != "" {
		return p.providerName
	}

	return p.integrationName
}

func (p *GeneratedSetupProvider) validateCapabilities(capabilities []string) error {
	valid := p.capabilitySet()
	for _, capability := range capabilities {
		if _, ok := valid[capability]; !ok {
			return fmt.Errorf("invalid capability %q", capability)
		}
	}
	return nil
}

func (p *GeneratedSetupProvider) allCapabilities() []string {
	var capabilities []string
	for _, group := range p.capabilityGroups {
		for _, capability := range group.Capabilities {
			capabilities = append(capabilities, capability.Name)
		}
	}
	return capabilities
}

func (p *GeneratedSetupProvider) capabilitySet() map[string]struct{} {
	valid := map[string]struct{}{}
	for _, capability := range p.allCapabilities() {
		valid[capability] = struct{}{}
	}
	return valid
}
