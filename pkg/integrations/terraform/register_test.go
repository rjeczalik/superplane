package terraform

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/crypto"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
	"github.com/superplanehq/superplane/pkg/registry"
)

func TestRegisterConfiguredProviders(t *testing.T) {
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers: []config.TerraformProviderIntegration{talosConfig()},
		RuntimeFactory: &fakeRuntimeFactory{
			runtime: &fakeProviderRuntime{schema: runtimeSchema(t)},
		},
		CacheDir: t.TempDir(),
		Logger:   log.NewEntry(log.New()),
	})
	require.NoError(t, err)

	r, err := registry.NewRegistryWithOptions(registry.RegistryOptions{Encryptor: crypto.NewNoOpEncryptor()})
	require.NoError(t, err)

	integration, err := r.GetIntegration("terraform_talos")
	require.NoError(t, err)
	assert.NotNil(t, integration)

	action, err := r.GetIntegrationAction("terraform_talos", "terraform_talos.machineSecrets.create")
	require.NoError(t, err)
	assert.NotNil(t, action)

	trigger, err := r.GetIntegrationTrigger("terraform_talos", "terraform_talos.machineSecrets.onChanged")
	require.NoError(t, err)
	assert.NotNil(t, trigger)
}

func TestRegisterConfiguredProviders_EmptyDoesNotCreateRuntimeFactory(t *testing.T) {
	called := false
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers: []config.TerraformProviderIntegration{},
		NewConfiguredRuntimeFactory: func(cacheDir string) (ConfiguredRuntimeFactory, error) {
			called = true
			return nil, errors.New("should not be called")
		},
	})
	require.NoError(t, err)
	assert.False(t, called)
}

func TestRegisterConfiguredProviders_RuntimeFactoryLoadsSchema(t *testing.T) {
	factory := &fakeRuntimeFactory{runtime: &fakeProviderRuntime{schema: runtimeSchema(t)}}
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers:      []config.TerraformProviderIntegration{talosConfig()},
		CacheDir:       t.TempDir(),
		Logger:         log.NewEntry(log.New()),
		RuntimeFactory: factory,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, factory.calls)
}

func TestRegisterConfiguredProviders_BuildsDefaultRuntimeFactory(t *testing.T) {
	factory := &fakeRuntimeFactory{runtime: &fakeProviderRuntime{schema: runtimeSchema(t)}}
	called := false
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers: []config.TerraformProviderIntegration{talosConfig()},
		CacheDir:  t.TempDir(),
		Logger:    log.NewEntry(log.New()),
		NewConfiguredRuntimeFactory: func(cacheDir string) (ConfiguredRuntimeFactory, error) {
			called = true
			return factory, nil
		},
	})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, 1, factory.calls)
}

func TestRegisterConfiguredProviders_ProductionRejectsWildcardResourceExposureBeforeSchemaLoad(t *testing.T) {
	called := false
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers: []config.TerraformProviderIntegration{talosConfig()},
		AppEnv:    "production",
		NewConfiguredRuntimeFactory: func(cacheDir string) (ConfiguredRuntimeFactory, error) {
			called = true
			return &fakeRuntimeFactory{runtime: &fakeProviderRuntime{schema: runtimeSchema(t)}}, nil
		},
		CacheDir: t.TempDir(),
		Logger:   log.NewEntry(log.New()),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expose.resources wildcard")
	assert.False(t, called)
}

func TestRegisterConfiguredProviders_AbsentResourceExposureRegistersNoResourceCRUD(t *testing.T) {
	cfg := talosConfig()
	cfg.Expose.Resources = []string{}
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers: []config.TerraformProviderIntegration{cfg},
		RuntimeFactory: &fakeRuntimeFactory{
			runtime: &fakeProviderRuntime{schema: runtimeSchema(t)},
		},
		CacheDir: t.TempDir(),
		Logger:   log.NewEntry(log.New()),
	})
	require.NoError(t, err)

	r, err := registry.NewRegistryWithOptions(registry.RegistryOptions{Encryptor: crypto.NewNoOpEncryptor()})
	require.NoError(t, err)
	_, err = r.GetIntegrationAction("terraform_talos", "terraform_talos.machineSecrets.create")
	require.Error(t, err)
}

func TestRegisterConfiguredProviders_RuntimeFactoryInjectsDirectRunner(t *testing.T) {
	schema := runtimeSchema(t)
	schema.DataSources = map[string]runtime.SchemaBlock{
		"talos_machine_configuration": {
			Version: 1,
			Block: blockJSON(t, Block{Attributes: map[string]Attribute{
				"cluster_name": {Type: rawJSON(t, `"string"`), Required: true},
				"machine_type": {Type: rawJSON(t, `"string"`), Required: true},
			}}),
		},
	}
	execRuntime := &fakeDirectRuntime{
		readDataSource: &runtime.ReadDataSourceResponse{State: runtime.DynamicValue{JSON: []byte(`{"id":"read"}`)}},
	}
	factory := &queuedRuntimeFactory{runtimes: []runtime.ProviderRuntime{
		&fakeProviderRuntime{schema: schema},
		execRuntime,
	}}
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers:      []config.TerraformProviderIntegration{talosConfig()},
		CacheDir:       t.TempDir(),
		Logger:         log.NewEntry(log.New()),
		RuntimeFactory: factory,
	})
	require.NoError(t, err)

	r, err := registry.NewRegistryWithOptions(registry.RegistryOptions{Encryptor: crypto.NewNoOpEncryptor()})
	require.NoError(t, err)
	action, err := r.GetIntegrationAction("terraform_talos", "terraform_talos.machineConfiguration.data")
	require.NoError(t, err)

	execState := &recordingExecutionState{}
	err = action.Execute(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"cluster_name": "cluster", "machine_type": "controlplane"},
		Integration: &runnerIntegrationContext{
			mockIntegrationContextWithCapabilities: mockIntegrationContextWithCapabilities{
				capabilities: []core.CapabilityState{{Name: "terraform_talos.machineConfiguration.data", State: core.IntegrationCapabilityStateEnabled}},
			},
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, factory.calls)
	assert.Equal(t, []string{"Configure", "ValidateDataSourceConfig", "ReadDataSource", "Close"}, execRuntime.calls)
	assert.Equal(t, "terraform.machineConfiguration.data", execState.payloadType)
}

func TestRegisterConfiguredProviders_ResourceCRUDUsesResourceRunner(t *testing.T) {
	execRuntime := &fakeDirectRuntime{
		createResource: &runtime.ResourceResult{
			NewState: runtime.ProviderState{Envelope: []byte(`{"id":"created"}`)},
		},
	}
	factory := &queuedRuntimeFactory{runtimes: []runtime.ProviderRuntime{
		&fakeProviderRuntime{schema: runtimeSchema(t)},
		execRuntime,
	}}
	store := &memoryManagedResourceStore{}
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers:            []config.TerraformProviderIntegration{talosConfig()},
		CacheDir:             t.TempDir(),
		Logger:               log.NewEntry(log.New()),
		RuntimeFactory:       factory,
		ManagedResourceStore: store,
	})
	require.NoError(t, err)

	r, err := registry.NewRegistryWithOptions(registry.RegistryOptions{Encryptor: crypto.NewNoOpEncryptor()})
	require.NoError(t, err)
	action, err := r.GetIntegrationAction("terraform_talos", "terraform_talos.machineSecrets.create")
	require.NoError(t, err)

	execState := &recordingExecutionState{}
	err = action.Execute(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"name": "machine"},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	})
	require.NoError(t, err)
	assert.True(t, store.beginCalled)
	assert.True(t, store.completeCalled)
	assert.Equal(t, []string{"Configure", "ValidateResourceConfig", "CreateResource", "Close"}, execRuntime.calls)
	assert.Equal(t, "terraform.machineSecrets.create", execState.payloadType)
}

func TestRegisterConfiguredProviders_BadConfig(t *testing.T) {
	t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{bad json]`)
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid TERRAFORM_PROVIDER_INTEGRATIONS")
}

func TestRegisterConfiguredProviders_DirectRuntimePreconditionError(t *testing.T) {
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers: []config.TerraformProviderIntegration{talosConfig()},
		NewConfiguredRuntimeFactory: func(cacheDir string) (ConfiguredRuntimeFactory, error) {
			return nil, errors.New("missing plugin cache")
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terraform direct runtime precondition failed")
}

func TestRegisterConfiguredProviders_LoadError(t *testing.T) {
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers:      []config.TerraformProviderIntegration{talosConfig()},
		RuntimeFactory: &fakeRuntimeFactory{runtime: &fakeProviderRuntime{schemaErr: errors.New("schema failed")}},
		CacheDir:       t.TempDir(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load talos")
}

type queuedRuntimeFactory struct {
	runtimes []runtime.ProviderRuntime
	calls    int
}

func (f *queuedRuntimeFactory) RuntimeForProvider(ctx context.Context, cfg config.TerraformProviderIntegration) (runtime.ProviderRuntime, error) {
	if f.calls >= len(f.runtimes) {
		return nil, errors.New("unexpected RuntimeForProvider call")
	}
	rt := f.runtimes[f.calls]
	f.calls++
	return rt, nil
}
