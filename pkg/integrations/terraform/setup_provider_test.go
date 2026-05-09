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

var _ core.IntegrationSetupProvider = (*GeneratedSetupProvider)(nil)
var _ core.ProductionSetupFlowProvider = (*GeneratedSetupProvider)(nil)

func fakeValidator(err error) TerraformValidator {
	return &fakeTerraformValidator{err: err}
}

type fakeTerraformValidator struct {
	err error
}

func (f *fakeTerraformValidator) Validate(ctx context.Context, providerName, providerSource, providerVersion string, providerConfig map[string]any) error {
	return f.err
}

func TestGeneratedSetupProvider_SupportsSetupFlowInProduction(t *testing.T) {
	s := &GeneratedSetupProvider{}
	assert.True(t, s.SupportsSetupFlowInProduction())
}

func TestGeneratedSetupProvider_CapabilityGroups(t *testing.T) {
	s := &GeneratedSetupProvider{
		capabilityGroups: []core.CapabilityGroup{
			{
				Label: "Resources",
				Capabilities: []core.Capability{
					{Name: "talos.machineSecrets.create", Type: core.IntegrationCapabilityTypeAction},
				},
			},
			{
				Label: "Data Sources",
				Capabilities: []core.Capability{
					{Name: "talos.machineConfiguration.data", Type: core.IntegrationCapabilityTypeAction},
				},
			},
		},
	}
	groups := s.CapabilityGroups()
	require.Len(t, groups, 2)
	assert.Equal(t, "Resources", groups[0].Label)
	assert.Equal(t, "Data Sources", groups[1].Label)
}

func TestGeneratedSetupProvider_FirstStep(t *testing.T) {
	s := newTestSetupProvider(nil)
	step := s.FirstStep(core.SetupStepContext{})
	assert.Equal(t, core.SetupStepTypeCapabilitySelection, step.Type)
	assert.Equal(t, setupStepCapabilitySelection, step.Name)
	require.Len(t, step.Capabilities, 2)
	assert.Contains(t, step.Capabilities, "talos.machineSecrets.create")
	assert.Contains(t, step.Capabilities, "talos.machineConfiguration.data")
}

func TestGeneratedSetupProvider_OnStepSubmit_CapabilitySelection(t *testing.T) {
	s := newTestSetupProvider(nil)

	ctx := core.SetupStepContext{
		Step: core.StepInfo{
			Name:         setupStepCapabilitySelection,
			Capabilities: []string{"talos.machineSecrets.create"},
		},
		Capabilities: &mockCapabilityContext{},
	}

	next, err := s.OnStepSubmit(ctx)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, core.SetupStepTypeInputs, next.Type)
	assert.Equal(t, setupStepProviderConfig, next.Name)
}

func TestGeneratedSetupProvider_OnStepSubmit_CapabilitySelection_EmptyProviderConfig(t *testing.T) {
	s := newTestSetupProviderWithSchema(nil, []configuration.Field{})

	mockCaps := &mockCapabilityContext{}
	ctx := core.SetupStepContext{
		Step: core.StepInfo{
			Name:         setupStepCapabilitySelection,
			Capabilities: []string{"talos.machineSecrets.create"},
		},
		Capabilities: mockCaps,
		Secrets:      &mockSecretStorage{},
	}

	next, err := s.OnStepSubmit(ctx)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, core.SetupStepTypeDone, next.Type)
	assert.Equal(t, setupStepDone, next.Name)
}

func TestGeneratedSetupProvider_OnStepSubmit_ProviderConfig(t *testing.T) {
	s := newTestSetupProvider(nil)

	mockCaps := &mockCapabilityContext{requested: []string{"talos.machineSecrets.create"}}
	mockSecrets := &mockSecretStorage{}
	ctx := core.SetupStepContext{
		Step: core.StepInfo{
			Name:   setupStepProviderConfig,
			Inputs: map[string]any{"endpoint": "https://example.com"},
		},
		Capabilities: mockCaps,
		Secrets:      mockSecrets,
	}

	next, err := s.OnStepSubmit(ctx)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, core.SetupStepTypeDone, next.Type)
	assert.Equal(t, setupStepDone, next.Name)
	assert.True(t, mockCaps.enabledCalled)
	assert.Equal(t, []string{"talos.machineSecrets.create"}, mockCaps.enabled)
}

func TestGeneratedSetupProvider_OnStepSubmit_ProviderConfig_DoesNotEnableUnrequestedCapabilities(t *testing.T) {
	s := newTestSetupProvider(nil)

	mockCaps := &mockCapabilityContext{}
	mockSecrets := &mockSecretStorage{}
	ctx := core.SetupStepContext{
		Step: core.StepInfo{
			Name:   setupStepProviderConfig,
			Inputs: map[string]any{"endpoint": "https://example.com"},
		},
		Capabilities: mockCaps,
		Secrets:      mockSecrets,
	}

	next, err := s.OnStepSubmit(ctx)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.False(t, mockCaps.enabledCalled)
	assert.Empty(t, mockCaps.enabled)
}

func TestGeneratedSetupProvider_OnStepSubmit_ProviderConfig_ValidationFails(t *testing.T) {
	s := newTestSetupProvider(fakeValidator(errors.New("bad config")))

	mockCaps := &mockCapabilityContext{}
	ctx := core.SetupStepContext{
		Step: core.StepInfo{
			Name:   setupStepProviderConfig,
			Inputs: map[string]any{"endpoint": "https://example.com"},
		},
		Capabilities: mockCaps,
		Secrets:      &mockSecretStorage{},
	}

	next, err := s.OnStepSubmit(ctx)
	require.Error(t, err)
	assert.Nil(t, next)
	assert.Contains(t, err.Error(), "bad config")
	assert.False(t, mockCaps.enabledCalled)
}

func TestGeneratedSetupProvider_OnStepSubmit_UnknownStep(t *testing.T) {
	s := newTestSetupProvider(nil)
	ctx := core.SetupStepContext{Step: core.StepInfo{Name: "unknown"}}
	_, err := s.OnStepSubmit(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown step")
}

func TestGeneratedSetupProvider_OnPropertyUpdate(t *testing.T) {
	s := newTestSetupProvider(nil)
	_, err := s.OnPropertyUpdate(core.PropertyUpdateContext{PropertyName: "anything"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestGeneratedSetupProvider_OnSecretUpdate_TerraformProviderConfig(t *testing.T) {
	s := newTestSetupProvider(nil)
	step, err := s.OnSecretUpdate(core.SecretUpdateContext{
		SecretName: secretNameTerraformProviderConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, step)
	assert.Equal(t, core.SetupStepTypeInputs, step.Type)
	assert.Equal(t, setupStepProviderConfig, step.Name)
}

func TestGeneratedSetupProvider_OnSecretUpdate_Unknown(t *testing.T) {
	s := newTestSetupProvider(nil)
	_, err := s.OnSecretUpdate(core.SecretUpdateContext{SecretName: "other"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown secret")
}

func TestGeneratedSetupProvider_OnCapabilityUpdate(t *testing.T) {
	s := newTestSetupProvider(nil)
	mockCaps := &mockCapabilityContext{}
	_, err := s.OnCapabilityUpdate(core.CapabilityUpdateContext{
		Changes: map[core.IntegrationCapabilityState][]string{
			core.IntegrationCapabilityStateRequested: {"talos.machineSecrets.create"},
		},
		Capabilities: mockCaps,
	})
	require.NoError(t, err)
	assert.True(t, mockCaps.enabledCalled)
}

func TestGeneratedSetupProvider_OnCapabilityUpdate_InvalidCapability(t *testing.T) {
	s := newTestSetupProvider(nil)
	_, err := s.OnCapabilityUpdate(core.CapabilityUpdateContext{
		Changes: map[core.IntegrationCapabilityState][]string{
			core.IntegrationCapabilityStateRequested: {"invalid.capability"},
		},
		Capabilities: &mockCapabilityContext{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid capability")
}

func TestGeneratedSetupProvider_OnStepRevert_CapabilitySelection(t *testing.T) {
	s := newTestSetupProvider(nil)
	mockCaps := &mockCapabilityContext{}
	err := s.OnStepRevert(core.SetupStepContext{
		Step:         core.StepInfo{Name: setupStepCapabilitySelection},
		Capabilities: mockCaps,
	})
	require.NoError(t, err)
	assert.True(t, mockCaps.clearedCalled)
}

func TestGeneratedSetupProvider_OnStepRevert_ProviderConfig(t *testing.T) {
	s := newTestSetupProvider(nil)
	mockSecrets := &mockSecretStorage{}
	err := s.OnStepRevert(core.SetupStepContext{
		Step:    core.StepInfo{Name: setupStepProviderConfig},
		Secrets: mockSecrets,
	})
	require.NoError(t, err)
	assert.True(t, mockSecrets.deletedCalled)
	assert.Equal(t, secretNameTerraformProviderConfig, mockSecrets.deletedName)
}

func TestGeneratedSetupProvider_OnStepRevert_UnknownStep(t *testing.T) {
	s := newTestSetupProvider(nil)
	err := s.OnStepRevert(core.SetupStepContext{Step: core.StepInfo{Name: "unknown"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown step")
}

// helpers

func newTestSetupProvider(validator TerraformValidator) *GeneratedSetupProvider {
	return newTestSetupProviderWithSchema(validator, []configuration.Field{
		{Name: "endpoint", Type: configuration.FieldTypeString, Required: true},
	})
}

func newTestSetupProviderWithSchema(validator TerraformValidator, schema []configuration.Field) *GeneratedSetupProvider {
	if validator == nil {
		validator = fakeValidator(nil)
	}
	return &GeneratedSetupProvider{
		integrationName: "talos",
		capabilityGroups: []core.CapabilityGroup{
			{
				Label: "Resources",
				Capabilities: []core.Capability{
					{Name: "talos.machineSecrets.create", Type: core.IntegrationCapabilityTypeAction},
				},
			},
			{
				Label: "Data Sources",
				Capabilities: []core.Capability{
					{Name: "talos.machineConfiguration.data", Type: core.IntegrationCapabilityTypeAction},
				},
			},
		},
		providerConfigSchema: schema,
		providerSource:       "registry.terraform.io/siderolabs/talos",
		providerVersion:      "0.11.0",
		validator:            validator,
	}
}

// mockCapabilityContext tracks calls for assertions
type mockCapabilityContext struct {
	requested     []string
	enabled       []string
	enabledCalled bool
	clearedCalled bool
}

func (m *mockCapabilityContext) Request(capabilities ...string) {
	m.requested = append(m.requested, capabilities...)
}
func (m *mockCapabilityContext) Available(capabilities ...string)   {}
func (m *mockCapabilityContext) Unavailable(capabilities ...string) {}
func (m *mockCapabilityContext) Enable(capabilities ...string) {
	m.enabledCalled = true
	m.enabled = append(m.enabled, capabilities...)
}
func (m *mockCapabilityContext) Disable(capabilities ...string) {}
func (m *mockCapabilityContext) Clear() {
	m.clearedCalled = true
}
func (m *mockCapabilityContext) IsRequested(capabilities ...string) bool {
	for _, c := range m.requested {
		for _, check := range capabilities {
			if c == check {
				return true
			}
		}
	}
	return false
}
func (m *mockCapabilityContext) Requested() []string {
	return m.requested
}
func (m *mockCapabilityContext) Enabled() []string {
	return m.enabled
}

// mockSecretStorage tracks calls for assertions
type mockSecretStorage struct {
	createdCalled bool
	deletedCalled bool
	deletedName   string
	secrets       map[string]string
	getErr        error
}

func (m *mockSecretStorage) Get(name string) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	if m.secrets == nil {
		return "", errors.New("not found")
	}
	v, ok := m.secrets[name]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (m *mockSecretStorage) Delete(name string) error {
	m.deletedCalled = true
	m.deletedName = name
	return nil
}
func (m *mockSecretStorage) Create(def core.IntegrationSecretDefinition) error {
	m.createdCalled = true
	if m.secrets == nil {
		m.secrets = make(map[string]string)
	}
	m.secrets[def.Name] = def.Value
	return nil
}
func (m *mockSecretStorage) CreateMany(defs []core.IntegrationSecretDefinition) error {
	for _, def := range defs {
		if err := m.Create(def); err != nil {
			return err
		}
	}
	return nil
}
func (m *mockSecretStorage) Update(name string, value string) error {
	if m.secrets == nil {
		m.secrets = make(map[string]string)
	}
	m.secrets[name] = value
	return nil
}
