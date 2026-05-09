package terraform

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

func TestGeneratedIntegration_Metadata(t *testing.T) {
	action := &GeneratedAction{integrationName: "talos", resourceName: "machineSecrets", op: "create"}
	setupProvider := &GeneratedSetupProvider{}
	i := &GeneratedIntegration{
		name:                 "talos",
		label:                "Talos",
		icon:                 "talos.svg",
		description:          "Talos provider",
		providerSource:       "registry.terraform.io/siderolabs/talos",
		providerVersion:      "0.11.0",
		providerConfigSchema: []configuration.Field{{Name: "endpoint", Type: configuration.FieldTypeString}},
		actions:              []core.Action{action},
		setupProvider:        setupProvider,
	}

	assert.Equal(t, "talos", i.Name())
	assert.Equal(t, "Talos", i.Label())
	assert.Equal(t, "talos.svg", i.Icon())
	assert.Equal(t, "Talos provider", i.Description())
	assert.NotEmpty(t, i.Instructions())
	assert.Len(t, i.Configuration(), 1)
	assert.Equal(t, []core.Action{action}, i.Actions())
	assert.Nil(t, i.Triggers())
	assert.Nil(t, i.Hooks())
	assert.Equal(t, "terraform", i.Origin())
	assert.Equal(t, "registry.terraform.io/siderolabs/talos", i.Source())
	assert.Equal(t, "0.11.0", i.Version())
	assert.Same(t, setupProvider, i.SetupProvider())
}

func TestGeneratedIntegration_ZeroEffectMethods(t *testing.T) {
	i := &GeneratedIntegration{}
	assert.NoError(t, i.Cleanup(core.IntegrationCleanupContext{}))
	assert.NoError(t, i.HandleHook(core.IntegrationHookContext{}))
	resources, err := i.ListResources("anything", core.ListResourcesContext{})
	assert.NoError(t, err)
	assert.Nil(t, resources)
}

func TestGeneratedIntegration_SyncSuccess(t *testing.T) {
	validator := &recordingTerraformValidator{}
	integrationCtx := &syncIntegrationContext{
		secrets: &mockSecretStorage{secrets: map[string]string{
			secretNameTerraformProviderConfig: `{"endpoint":"https://example.com"}`,
		}},
	}
	i := &GeneratedIntegration{
		name:            "talos",
		providerSource:  "registry.terraform.io/siderolabs/talos",
		providerVersion: "0.11.0",
		validator:       validator,
	}

	err := i.Sync(core.SyncContext{Integration: integrationCtx})
	require.NoError(t, err)
	assert.True(t, integrationCtx.readyCalled)
	assert.False(t, integrationCtx.errorCalled)
	assert.Equal(t, "talos", validator.providerName)
	assert.Equal(t, "registry.terraform.io/siderolabs/talos", validator.providerSource)
	assert.Equal(t, "0.11.0", validator.providerVersion)
	assert.Equal(t, "https://example.com", validator.providerConfig["endpoint"])
}

func TestGeneratedIntegration_SyncValidationFailure(t *testing.T) {
	integrationCtx := &syncIntegrationContext{
		secrets: &mockSecretStorage{secrets: map[string]string{
			secretNameTerraformProviderConfig: `{}`,
		}},
	}
	i := &GeneratedIntegration{
		name:            "talos",
		providerSource:  "registry.terraform.io/siderolabs/talos",
		providerVersion: "0.11.0",
		validator:       &recordingTerraformValidator{err: errors.New("bad config")},
	}

	err := i.Sync(core.SyncContext{Integration: integrationCtx})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad config")
	assert.False(t, integrationCtx.readyCalled)
	assert.True(t, integrationCtx.errorCalled)
}

func TestGeneratedIntegration_SyncMissingSecretUsesEmptyConfig(t *testing.T) {
	validator := &recordingTerraformValidator{}
	integrationCtx := &syncIntegrationContext{secrets: &mockSecretStorage{}}
	i := &GeneratedIntegration{name: "talos", validator: validator}

	err := i.Sync(core.SyncContext{Integration: integrationCtx})
	require.NoError(t, err)
	assert.Empty(t, validator.providerConfig)
	assert.True(t, integrationCtx.readyCalled)
}

type recordingTerraformValidator struct {
	err             error
	providerName    string
	providerSource  string
	providerVersion string
	providerConfig  map[string]any
}

func (v *recordingTerraformValidator) Validate(ctx context.Context, providerName, providerSource, providerVersion string, providerConfig map[string]any) error {
	v.providerName = providerName
	v.providerSource = providerSource
	v.providerVersion = providerVersion
	v.providerConfig = providerConfig
	return v.err
}

type syncIntegrationContext struct {
	mockIntegrationContextNoCapabilities
	secrets     core.IntegrationSecretStorage
	readyCalled bool
	errorCalled bool
}

func (c *syncIntegrationContext) Secrets() core.IntegrationSecretStorage {
	return c.secrets
}

func (c *syncIntegrationContext) Ready() {
	c.readyCalled = true
}

func (c *syncIntegrationContext) Error(message string) {
	c.errorCalled = true
}
