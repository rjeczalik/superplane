package terraform

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

// Compile-time interface checks
var _ core.Action = (*GeneratedAction)(nil)

func TestGeneratedAction_Name(t *testing.T) {
	a := &GeneratedAction{
		integrationName: "talos",
		resourceName:    "machineSecrets",
		op:              "create",
	}
	assert.Equal(t, "talos.machineSecrets.create", a.Name())
}

func TestGeneratedAction_Label(t *testing.T) {
	tests := []struct {
		name         string
		resourceName string
		op           string
		want         string
	}{
		{
			name:         "create prefixes create",
			resourceName: "machineSecrets",
			op:           "create",
			want:         "Create Machine Secrets",
		},
		{
			name:         "read prefixes get",
			resourceName: "machineConfiguration",
			op:           "read",
			want:         "Get Machine Configuration",
		},
		{
			name:         "provider action uses base label",
			resourceName: "restartMachine",
			op:           "action",
			want:         "Restart Machine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &GeneratedAction{resourceName: tt.resourceName, op: tt.op}
			assert.Equal(t, tt.want, a.Label())
		})
	}
}

func TestGeneratedAction_Configuration(t *testing.T) {
	fields := []configuration.Field{
		{Name: "node", Type: configuration.FieldTypeString, Required: true},
	}
	a := &GeneratedAction{
		inputSchema: fields,
	}
	assert.Equal(t, fields, a.Configuration())
}

func TestGeneratedAction_OutputChannels(t *testing.T) {
	a := &GeneratedAction{}
	channels := a.OutputChannels(nil)
	require.Len(t, channels, 1)
	assert.Equal(t, core.DefaultOutputChannel.Name, channels[0].Name)
}

func TestGeneratedAction_ProcessQueueItem_UsesDefaultProcessing(t *testing.T) {
	executionID := uuid.New()
	called := false
	a := &GeneratedAction{}

	got, err := a.ProcessQueueItem(core.ProcessQueueContext{
		DefaultProcessing: func() (*uuid.UUID, error) {
			called = true
			return &executionID, nil
		},
	})

	require.NoError(t, err)
	require.True(t, called)
	require.NotNil(t, got)
	assert.Equal(t, executionID, *got)
}

func TestGeneratedAction_Setup_Cleanup_Cancel_HandleHook_HandleWebhook(t *testing.T) {
	a := &GeneratedAction{}
	ctx := core.SetupContext{}
	assert.NoError(t, a.Setup(ctx))
	assert.NoError(t, a.Cleanup(ctx))

	execCtx := core.ExecutionContext{}
	assert.NoError(t, a.Cancel(execCtx))
	assert.NoError(t, a.HandleHook(core.ActionHookContext{}))

	code, resp, err := a.HandleWebhook(core.WebhookRequestContext{})
	assert.Equal(t, 0, code)
	assert.Nil(t, resp)
	assert.NoError(t, err)
}

func TestGeneratedAction_Hooks(t *testing.T) {
	a := &GeneratedAction{}
	assert.Nil(t, a.Hooks())
}

func TestGeneratedAction_Documentation(t *testing.T) {
	a := &GeneratedAction{
		integrationName: "talos",
		resourceName:    "machineSecrets",
		op:              "create",
		description:     "Create machine secrets.",
	}
	doc := a.Documentation()
	assert.Contains(t, doc, "machineSecrets")
	assert.Contains(t, doc, "Create machine secrets")
}

func TestGeneratedAction_Getters(t *testing.T) {
	a := &GeneratedAction{
		capabilityName:  "talos.machineSecrets.create",
		schemaHash:      "abc123",
		providerName:    "talos",
		providerSource:  "registry.terraform.io/siderolabs/talos",
		providerVersion: "0.11.0",
	}
	assert.Equal(t, "talos.machineSecrets.create", a.CapabilityName())
	assert.Equal(t, "abc123", a.SchemaHash())
	assert.Equal(t, "talos", a.ProviderName())
	assert.Equal(t, "registry.terraform.io/siderolabs/talos", a.ProviderSource())
	assert.Equal(t, "0.11.0", a.ProviderVersion())
}

func TestGeneratedAction_Execute_NotImplemented(t *testing.T) {
	a := &GeneratedAction{
		capabilityName: "talos.machineSecrets.create",
	}
	execCtx := core.ExecutionContext{}
	err := a.Execute(execCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terraform runner not implemented")
}

func TestGeneratedAction_Execute_CapabilityGuard_Disabled(t *testing.T) {
	a := &GeneratedAction{
		capabilityName: "talos.machineSecrets.create",
	}

	mockInteg := &mockIntegrationContextWithCapabilities{
		capabilities: []core.CapabilityState{
			{Name: "talos.machineSecrets.create", State: core.IntegrationCapabilityStateDisabled},
		},
	}

	execCtx := core.ExecutionContext{Integration: mockInteg}
	err := a.Execute(execCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability \"talos.machineSecrets.create\" is not enabled")
}

func TestGeneratedAction_Execute_CapabilityGuard_Enabled(t *testing.T) {
	a := &GeneratedAction{
		capabilityName: "talos.machineSecrets.create",
	}

	mockInteg := &mockIntegrationContextWithCapabilities{
		capabilities: []core.CapabilityState{
			{Name: "talos.machineSecrets.create", State: core.IntegrationCapabilityStateEnabled},
		},
	}

	execCtx := core.ExecutionContext{Integration: mockInteg}
	err := a.Execute(execCtx)
	require.Error(t, err)
	// When enabled, it falls through to the not-implemented error
	assert.Contains(t, err.Error(), "terraform runner not implemented")
}

func TestGeneratedAction_Execute_CapabilityGuard_MissingCapability(t *testing.T) {
	a := &GeneratedAction{
		capabilityName: "talos.machineSecrets.create",
	}

	mockInteg := &mockIntegrationContextWithCapabilities{
		capabilities: []core.CapabilityState{
			{Name: "other.capability", State: core.IntegrationCapabilityStateEnabled},
		},
	}

	execCtx := core.ExecutionContext{Integration: mockInteg}
	err := a.Execute(execCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability \"talos.machineSecrets.create\" is not enabled")
}

func TestGeneratedAction_Execute_NoCapabilityAccessor(t *testing.T) {
	a := &GeneratedAction{
		capabilityName: "talos.machineSecrets.create",
	}

	// Integration that does NOT implement CapabilityStateAccessor
	mockInteg := &mockIntegrationContextNoCapabilities{}

	execCtx := core.ExecutionContext{Integration: mockInteg}
	err := a.Execute(execCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terraform runner not implemented")
}

// mockIntegrationContextWithCapabilities implements core.IntegrationContext and core.CapabilityStateAccessor
type mockIntegrationContextWithCapabilities struct {
	capabilities []core.CapabilityState
}

func (m *mockIntegrationContextWithCapabilities) LegacySetup() bool { return false }
func (m *mockIntegrationContextWithCapabilities) Properties() core.IntegrationPropertyStorage {
	return nil
}
func (m *mockIntegrationContextWithCapabilities) Secrets() core.IntegrationSecretStorage { return nil }
func (m *mockIntegrationContextWithCapabilities) ID() uuid.UUID                          { return uuid.Nil }
func (m *mockIntegrationContextWithCapabilities) GetMetadata() any                       { return nil }
func (m *mockIntegrationContextWithCapabilities) SetMetadata(any)                        {}
func (m *mockIntegrationContextWithCapabilities) GetConfig(name string) ([]byte, error) {
	return nil, nil
}
func (m *mockIntegrationContextWithCapabilities) Ready()                                     {}
func (m *mockIntegrationContextWithCapabilities) Error(message string)                       {}
func (m *mockIntegrationContextWithCapabilities) NewBrowserAction(action core.BrowserAction) {}
func (m *mockIntegrationContextWithCapabilities) RemoveBrowserAction()                       {}
func (m *mockIntegrationContextWithCapabilities) SetSecret(name string, value []byte) error {
	return nil
}
func (m *mockIntegrationContextWithCapabilities) GetSecrets() ([]core.IntegrationSecret, error) {
	return nil, nil
}
func (m *mockIntegrationContextWithCapabilities) RequestWebhook(configuration any) error { return nil }
func (m *mockIntegrationContextWithCapabilities) Subscribe(any) (*uuid.UUID, error)      { return nil, nil }
func (m *mockIntegrationContextWithCapabilities) ScheduleResync(interval time.Duration) error {
	return nil
}
func (m *mockIntegrationContextWithCapabilities) ScheduleActionCall(actionName string, parameters any, interval time.Duration) error {
	return nil
}
func (m *mockIntegrationContextWithCapabilities) ListSubscriptions() ([]core.IntegrationSubscriptionContext, error) {
	return nil, nil
}
func (m *mockIntegrationContextWithCapabilities) FindSubscription(predicate func(core.IntegrationSubscriptionContext) bool) (core.IntegrationSubscriptionContext, error) {
	return nil, nil
}
func (m *mockIntegrationContextWithCapabilities) Capabilities() []core.CapabilityState {
	return m.capabilities
}

// mockIntegrationContextNoCapabilities implements core.IntegrationContext but NOT core.CapabilityStateAccessor
type mockIntegrationContextNoCapabilities struct{}

func (m *mockIntegrationContextNoCapabilities) LegacySetup() bool { return false }
func (m *mockIntegrationContextNoCapabilities) Properties() core.IntegrationPropertyStorage {
	return nil
}
func (m *mockIntegrationContextNoCapabilities) Secrets() core.IntegrationSecretStorage { return nil }
func (m *mockIntegrationContextNoCapabilities) ID() uuid.UUID                          { return uuid.Nil }
func (m *mockIntegrationContextNoCapabilities) GetMetadata() any                       { return nil }
func (m *mockIntegrationContextNoCapabilities) SetMetadata(any)                        {}
func (m *mockIntegrationContextNoCapabilities) GetConfig(name string) ([]byte, error) {
	return nil, nil
}
func (m *mockIntegrationContextNoCapabilities) Ready()                                     {}
func (m *mockIntegrationContextNoCapabilities) Error(message string)                       {}
func (m *mockIntegrationContextNoCapabilities) NewBrowserAction(action core.BrowserAction) {}
func (m *mockIntegrationContextNoCapabilities) RemoveBrowserAction()                       {}
func (m *mockIntegrationContextNoCapabilities) SetSecret(name string, value []byte) error  { return nil }
func (m *mockIntegrationContextNoCapabilities) GetSecrets() ([]core.IntegrationSecret, error) {
	return nil, nil
}
func (m *mockIntegrationContextNoCapabilities) RequestWebhook(configuration any) error { return nil }
func (m *mockIntegrationContextNoCapabilities) Subscribe(any) (*uuid.UUID, error)      { return nil, nil }
func (m *mockIntegrationContextNoCapabilities) ScheduleResync(interval time.Duration) error {
	return nil
}
func (m *mockIntegrationContextNoCapabilities) ScheduleActionCall(actionName string, parameters any, interval time.Duration) error {
	return nil
}
func (m *mockIntegrationContextNoCapabilities) ListSubscriptions() ([]core.IntegrationSubscriptionContext, error) {
	return nil, nil
}
func (m *mockIntegrationContextNoCapabilities) FindSubscription(predicate func(core.IntegrationSubscriptionContext) bool) (core.IntegrationSubscriptionContext, error) {
	return nil, nil
}
