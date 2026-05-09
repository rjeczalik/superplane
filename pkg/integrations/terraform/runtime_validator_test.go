package terraform

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestRuntimeValidatorValidateCallsProviderLifecycle(t *testing.T) {
	rt := &fakeProviderRuntime{
		prepared: runtime.DynamicValue{JSON: []byte(`{"token":"prepared"}`)},
	}
	factory := &fakeRuntimeFactory{runtime: rt}
	validator := NewRuntimeValidator(factory)

	err := validator.Validate(context.Background(), "talos", "registry.terraform.io/siderolabs/talos", "0.11.0", map[string]any{"token": "raw"})
	require.NoError(t, err)

	assert.Equal(t, 1, factory.calls)
	assert.Equal(t, []string{"prepare", "validate", "configure", "close"}, rt.calls)
	assert.JSONEq(t, `{"token":"prepared"}`, string(rt.validated.JSON))
	assert.JSONEq(t, `{"token":"prepared"}`, string(rt.configured.JSON))
}

func TestRuntimeValidatorValidateClosesRuntimeOnError(t *testing.T) {
	rt := &fakeProviderRuntime{validateErr: assert.AnError}
	err := NewRuntimeValidator(&fakeRuntimeFactory{runtime: rt}).Validate(context.Background(), "talos", "registry.terraform.io/siderolabs/talos", "0.11.0", map[string]any{})
	require.Error(t, err)
	assert.Equal(t, []string{"prepare", "validate", "close"}, rt.calls)
}

type fakeProviderRuntime struct {
	schema      *runtime.SchemaResponse
	schemaErr   error
	prepared    runtime.DynamicValue
	validated   runtime.DynamicValue
	configured  runtime.DynamicValue
	validateErr error
	calls       []string
	closed      bool
}

func (r *fakeProviderRuntime) Schema(ctx context.Context) (*runtime.SchemaResponse, error) {
	r.calls = append(r.calls, "schema")
	return r.schema, r.schemaErr
}

func (r *fakeProviderRuntime) PrepareProviderConfig(ctx context.Context, req *runtime.PrepareProviderConfigRequest) (*runtime.PrepareProviderConfigResponse, error) {
	r.calls = append(r.calls, "prepare")
	prepared := req.Config
	if len(r.prepared.JSON) > 0 {
		prepared = r.prepared
	}
	return &runtime.PrepareProviderConfigResponse{PreparedConfig: prepared}, nil
}

func (r *fakeProviderRuntime) ValidateProviderConfig(ctx context.Context, req *runtime.ValidateProviderConfigRequest) error {
	r.calls = append(r.calls, "validate")
	r.validated = req.Config
	return r.validateErr
}

func (r *fakeProviderRuntime) Configure(ctx context.Context, req *runtime.ConfigureRequest) error {
	r.calls = append(r.calls, "configure")
	r.configured = req.Config
	return nil
}

func (r *fakeProviderRuntime) ValidateDataSourceConfig(context.Context, *runtime.ValidateDataSourceConfigRequest) error {
	return nil
}
func (r *fakeProviderRuntime) ReadDataSource(context.Context, *runtime.ReadDataSourceRequest) (*runtime.ReadDataSourceResponse, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) ValidateAction(context.Context, *runtime.ValidateActionRequest) error {
	return nil
}
func (r *fakeProviderRuntime) PlanAction(context.Context, *runtime.PlanActionRequest) (*runtime.PlanActionResponse, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) InvokeAction(context.Context, *runtime.InvokeActionRequest) (*runtime.InvokeActionResponse, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) ValidateResourceConfig(context.Context, *runtime.ValidateResourceConfigRequest) error {
	return nil
}
func (r *fakeProviderRuntime) ReadResource(context.Context, *runtime.ReadResourceRequest) (*runtime.ReadResourceResponse, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) PlanResourceChange(context.Context, *runtime.PlanResourceChangeRequest) (*runtime.PlanResourceChangeResponse, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) ApplyResourceChange(context.Context, *runtime.ApplyResourceChangeRequest) (*runtime.ApplyResourceChangeResponse, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) CreateResource(context.Context, *runtime.CreateResourceRequest) (*runtime.ResourceResult, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) ReadResourceState(context.Context, *runtime.ReadResourceStateRequest) (*runtime.ResourceResult, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) UpdateResource(context.Context, *runtime.UpdateResourceRequest) (*runtime.ResourceResult, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) DeleteResource(context.Context, *runtime.DeleteResourceRequest) (*runtime.ResourceResult, error) {
	return nil, nil
}
func (r *fakeProviderRuntime) Close() error {
	r.calls = append(r.calls, "close")
	r.closed = true
	return nil
}
