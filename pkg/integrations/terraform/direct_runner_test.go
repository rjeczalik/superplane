package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestDirectRunner_ExecuteReadUsesProviderRuntime(t *testing.T) {
	rt := &fakeDirectRuntime{
		readDataSource: &runtime.ReadDataSourceResponse{State: runtime.DynamicValue{JSON: []byte(`{"id":"abc","password":"secret"}`)}},
	}
	execState := &recordingExecutionState{}
	integration := &runnerIntegrationContext{
		mockIntegrationContextWithCapabilities: mockIntegrationContextWithCapabilities{
			capabilities: []core.CapabilityState{{Name: "talos.machineSecrets.data", State: core.IntegrationCapabilityStateEnabled}},
		},
		secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
	}
	action := runnerTestAction()
	action.op = "data"
	action.capabilityName = "talos.machineSecrets.data"
	runner := NewDirectRunner(func(ctx context.Context, spec *runtime.ExecutionSpec) (runtime.ProviderRuntime, error) {
		assert.Equal(t, "talos.machineSecrets.data", spec.CapabilityName)
		assert.Equal(t, runtime.OpData, spec.Operation)
		return rt, nil
	}, 0)

	err := runner.Execute(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"machine_type": "controlplane"},
		Integration:    integration,
		ExecutionState: execState,
	}, action)
	require.NoError(t, err)

	assert.Equal(t, []string{"Configure", "ValidateDataSourceConfig", "ReadDataSource", "Close"}, rt.calls)
	assert.Equal(t, "talos_machine_secrets", rt.readDataSourceType)
	assert.Equal(t, core.DefaultOutputChannel.Name, execState.channel)
	assert.Equal(t, "terraform.machineSecrets.data", execState.payloadType)
	payload := execState.payloads[0].(map[string]any)
	assert.Equal(t, "abc", payload["id"])
	assert.Contains(t, payload["password"], "$terraformIntegrationSecret")
	assert.NotEmpty(t, integration.setSecrets)
}

func TestDirectRunner_ExecuteActionPlansWhenRequired(t *testing.T) {
	rt := &fakeDirectRuntime{
		planAction:   &runtime.PlanActionResponse{},
		invokeAction: &runtime.InvokeActionResponse{Events: []runtime.ActionEvent{{Type: "progress", Message: "started"}}},
	}
	execState := &recordingExecutionState{}
	action := runnerTestAction()
	action.op = "action"
	action.capabilityName = "talos.machineSecrets.action"
	action.hasPlanStep = true
	runner := NewDirectRunner(func(ctx context.Context, spec *runtime.ExecutionSpec) (runtime.ProviderRuntime, error) {
		return rt, nil
	}, 0)

	err := runner.Execute(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{},
		Integration: &runnerIntegrationContext{
			mockIntegrationContextWithCapabilities: mockIntegrationContextWithCapabilities{
				capabilities: []core.CapabilityState{{Name: "talos.machineSecrets.action", State: core.IntegrationCapabilityStateEnabled}},
			},
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	}, action)
	require.NoError(t, err)

	assert.Equal(t, []string{"Configure", "ValidateAction", "PlanAction", "InvokeAction", "Close"}, rt.calls)
	payload := execState.payloads[0].(map[string]any)
	events := payload["events"].([]map[string]any)
	assert.Equal(t, "started", events[0]["message"])
}

func TestDirectRunner_ClosesProviderOnError(t *testing.T) {
	rt := &fakeDirectRuntime{validateDataSourceErr: errors.New("invalid")}
	action := runnerTestAction()
	action.op = "data"
	action.capabilityName = "talos.machineSecrets.data"
	runner := NewDirectRunner(func(ctx context.Context, spec *runtime.ExecutionSpec) (runtime.ProviderRuntime, error) {
		return rt, nil
	}, 0)

	err := runner.Execute(core.ExecutionContext{
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Integration: &runnerIntegrationContext{
			mockIntegrationContextWithCapabilities: mockIntegrationContextWithCapabilities{
				capabilities: []core.CapabilityState{{Name: "talos.machineSecrets.data", State: core.IntegrationCapabilityStateEnabled}},
			},
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: &recordingExecutionState{},
	}, action)
	require.Error(t, err)
	assert.Equal(t, []string{"Configure", "ValidateDataSourceConfig", "Close"}, rt.calls)
}

func TestDirectRunner_AuditDoesNotLogPayloadValues(t *testing.T) {
	logger, hook := test.NewNullLogger()
	rt := &fakeDirectRuntime{
		readDataSource: &runtime.ReadDataSourceResponse{State: runtime.DynamicValue{JSON: []byte(`{"id":"abc","password":"secret-token"}`)}},
	}
	action := runnerTestAction()
	action.op = "data"
	action.capabilityName = "talos.machineSecrets.data"
	runner := NewDirectRunner(func(ctx context.Context, spec *runtime.ExecutionSpec) (runtime.ProviderRuntime, error) {
		return rt, nil
	}, 0, WithDirectRunnerAudit(NewAuditLogger(log.NewEntry(logger))))

	err := runner.Execute(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"password": "secret-token"},
		Integration: &runnerIntegrationContext{
			mockIntegrationContextWithCapabilities: mockIntegrationContextWithCapabilities{
				capabilities: []core.CapabilityState{{Name: "talos.machineSecrets.data", State: core.IntegrationCapabilityStateEnabled}},
			},
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{"token":"secret-token"}`}},
		},
		ExecutionState: &recordingExecutionState{},
	}, action)
	require.NoError(t, err)

	require.NotEmpty(t, hook.AllEntries())
	for _, entry := range hook.AllEntries() {
		serialized := entry.Message
		for key, value := range entry.Data {
			serialized += " " + key + "=" + strings.TrimSpace(toString(value))
		}
		assert.NotContains(t, serialized, "secret-token")
		assert.NotContains(t, serialized, "password")
	}
}

func marshalStateEnvelope(t *testing.T, value map[string]any) []byte {
	t.Helper()
	rawValue, err := json.Marshal(value)
	require.NoError(t, err)
	raw, err := json.Marshal(runtime.StateEnvelope{
		FormatVersion: 1,
		Protocol:      "tfplugin6",
		TypeName:      "talos_machine_secrets",
		SchemaVersion: 1,
		Value:         runtime.DynamicValue{JSON: rawValue},
	})
	require.NoError(t, err)
	return raw
}

type fakeDirectRuntime struct {
	calls                 []string
	readDataSourceType    string
	readDataSource        *runtime.ReadDataSourceResponse
	readResource          *runtime.ReadResourceResponse
	planResource          *runtime.PlanResourceChangeResponse
	applyResource         *runtime.ApplyResourceChangeResponse
	createResource        *runtime.ResourceResult
	readResourceState     *runtime.ResourceResult
	updateResource        *runtime.ResourceResult
	deleteResource        *runtime.ResourceResult
	planAction            *runtime.PlanActionResponse
	invokeAction          *runtime.InvokeActionResponse
	validateDataSourceErr error
	readResourceStateErr  error
}

func (r *fakeDirectRuntime) record(call string) {
	r.calls = append(r.calls, call)
}

func (r *fakeDirectRuntime) Schema(ctx context.Context) (*runtime.SchemaResponse, error) {
	r.record("Schema")
	return &runtime.SchemaResponse{}, nil
}

func (r *fakeDirectRuntime) PrepareProviderConfig(ctx context.Context, req *runtime.PrepareProviderConfigRequest) (*runtime.PrepareProviderConfigResponse, error) {
	r.record("PrepareProviderConfig")
	return &runtime.PrepareProviderConfigResponse{PreparedConfig: req.Config}, nil
}

func (r *fakeDirectRuntime) ValidateProviderConfig(ctx context.Context, req *runtime.ValidateProviderConfigRequest) error {
	r.record("ValidateProviderConfig")
	return nil
}

func (r *fakeDirectRuntime) Configure(ctx context.Context, req *runtime.ConfigureRequest) error {
	r.record("Configure")
	return nil
}

func (r *fakeDirectRuntime) ValidateDataSourceConfig(ctx context.Context, req *runtime.ValidateDataSourceConfigRequest) error {
	r.record("ValidateDataSourceConfig")
	return r.validateDataSourceErr
}

func (r *fakeDirectRuntime) ReadDataSource(ctx context.Context, req *runtime.ReadDataSourceRequest) (*runtime.ReadDataSourceResponse, error) {
	r.record("ReadDataSource")
	r.readDataSourceType = req.TypeName
	if r.readDataSource != nil {
		return r.readDataSource, nil
	}
	return &runtime.ReadDataSourceResponse{}, nil
}

func (r *fakeDirectRuntime) ValidateAction(ctx context.Context, req *runtime.ValidateActionRequest) error {
	r.record("ValidateAction")
	return nil
}

func (r *fakeDirectRuntime) PlanAction(ctx context.Context, req *runtime.PlanActionRequest) (*runtime.PlanActionResponse, error) {
	r.record("PlanAction")
	if r.planAction != nil {
		return r.planAction, nil
	}
	return &runtime.PlanActionResponse{}, nil
}

func (r *fakeDirectRuntime) InvokeAction(ctx context.Context, req *runtime.InvokeActionRequest) (*runtime.InvokeActionResponse, error) {
	r.record("InvokeAction")
	if r.invokeAction != nil {
		return r.invokeAction, nil
	}
	return &runtime.InvokeActionResponse{}, nil
}

func (r *fakeDirectRuntime) ValidateResourceConfig(ctx context.Context, req *runtime.ValidateResourceConfigRequest) error {
	r.record("ValidateResourceConfig")
	return nil
}

func (r *fakeDirectRuntime) ReadResource(ctx context.Context, req *runtime.ReadResourceRequest) (*runtime.ReadResourceResponse, error) {
	r.record("ReadResource")
	if r.readResource != nil {
		return r.readResource, nil
	}
	return &runtime.ReadResourceResponse{}, nil
}

func (r *fakeDirectRuntime) PlanResourceChange(ctx context.Context, req *runtime.PlanResourceChangeRequest) (*runtime.PlanResourceChangeResponse, error) {
	r.record("PlanResourceChange")
	if r.planResource != nil {
		return r.planResource, nil
	}
	return &runtime.PlanResourceChangeResponse{}, nil
}

func (r *fakeDirectRuntime) ApplyResourceChange(ctx context.Context, req *runtime.ApplyResourceChangeRequest) (*runtime.ApplyResourceChangeResponse, error) {
	r.record("ApplyResourceChange")
	if r.applyResource != nil {
		return r.applyResource, nil
	}
	return &runtime.ApplyResourceChangeResponse{}, nil
}

func (r *fakeDirectRuntime) CreateResource(ctx context.Context, req *runtime.CreateResourceRequest) (*runtime.ResourceResult, error) {
	r.record("CreateResource")
	if r.createResource != nil {
		return r.createResource, nil
	}
	return &runtime.ResourceResult{}, nil
}

func (r *fakeDirectRuntime) ReadResourceState(ctx context.Context, req *runtime.ReadResourceStateRequest) (*runtime.ResourceResult, error) {
	r.record("ReadResourceState")
	if r.readResourceStateErr != nil {
		return nil, r.readResourceStateErr
	}
	if r.readResourceState != nil {
		return r.readResourceState, nil
	}
	return &runtime.ResourceResult{}, nil
}

func (r *fakeDirectRuntime) UpdateResource(ctx context.Context, req *runtime.UpdateResourceRequest) (*runtime.ResourceResult, error) {
	r.record("UpdateResource")
	if r.updateResource != nil {
		return r.updateResource, nil
	}
	return &runtime.ResourceResult{}, nil
}

func (r *fakeDirectRuntime) DeleteResource(ctx context.Context, req *runtime.DeleteResourceRequest) (*runtime.ResourceResult, error) {
	r.record("DeleteResource")
	if r.deleteResource != nil {
		return r.deleteResource, nil
	}
	return &runtime.ResourceResult{}, nil
}

func (r *fakeDirectRuntime) Close() error {
	r.record("Close")
	return nil
}
