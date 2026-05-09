package v5

import (
	"context"
	"encoding/json"
	"iter"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestV5AdapterSchema(t *testing.T) {
	adapter := NewV5Adapter(newFakeV5Provider())

	schema, err := adapter.Schema(context.Background())
	require.NoError(t, err)

	assert.NotEmpty(t, schema.Provider.Block)
	assert.Equal(t, int64(2), schema.Resources["example_resource"].Version)
	assert.Contains(t, string(schema.Resources["example_resource"].Block), `"id"`)
	assert.Contains(t, string(schema.DataSources["example_data"].Block), `"name"`)
	assert.Contains(t, string(schema.Actions["example_action"].Block), `"name"`)
}

func TestV5AdapterSchemaToleratesUnimplementedResourceIdentitySchemas(t *testing.T) {
	provider := newFakeV5Provider()
	provider.identitySchemasErr = status.Error(codes.Unimplemented, "unknown method GetResourceIdentitySchemas")
	adapter := NewV5Adapter(provider)

	schema, err := adapter.Schema(context.Background())
	require.NoError(t, err)
	assert.Contains(t, schema.Resources, "example_resource")
}

func TestV5AdapterConfigMethods(t *testing.T) {
	provider := newFakeV5Provider()
	adapter := NewV5Adapter(provider)
	_, err := adapter.Schema(context.Background())
	require.NoError(t, err)

	prepared, err := adapter.PrepareProviderConfig(context.Background(), &runtime.PrepareProviderConfigRequest{
		Config: runtime.DynamicValue{JSON: []byte(`{"token":"abc"}`)},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"token":"prepared"}`, string(prepared.PreparedConfig.JSON))

	err = adapter.Configure(context.Background(), &runtime.ConfigureRequest{Config: runtime.DynamicValue{JSON: []byte(`{"token":"abc"}`)}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"token":"abc"}`, string(provider.configured.JSON))
}

func TestV5AdapterConfigMethodsHydrateSchemaLazily(t *testing.T) {
	provider := newFakeV5Provider()
	adapter := NewV5Adapter(provider)

	err := adapter.Configure(context.Background(), &runtime.ConfigureRequest{Config: runtime.DynamicValue{JSON: []byte(`{"token":"abc"}`)}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"token":"abc"}`, string(provider.configured.JSON))
	assert.Equal(t, 1, provider.schemaCalls)
}

func TestV5AdapterDataSource(t *testing.T) {
	adapter := NewV5Adapter(newFakeV5Provider())
	_, err := adapter.Schema(context.Background())
	require.NoError(t, err)

	err = adapter.ValidateDataSourceConfig(context.Background(), &runtime.ValidateDataSourceConfigRequest{
		TypeName: "example_data",
		Config:   runtime.DynamicValue{JSON: []byte(`{"name":"input"}`)},
	})
	require.NoError(t, err)

	resp, err := adapter.ReadDataSource(context.Background(), &runtime.ReadDataSourceRequest{
		TypeName: "example_data",
		Config:   runtime.DynamicValue{JSON: []byte(`{"name":"input"}`)},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"from-data"}`, string(resp.State.JSON))
}

func TestV5AdapterResourceLifecycle(t *testing.T) {
	adapter := NewV5Adapter(newFakeV5Provider())
	_, err := adapter.Schema(context.Background())
	require.NoError(t, err)

	initial := providerState(t, "example_resource", `{"id":"old","name":"before"}`, []byte("private-before"), 2)

	read, err := adapter.ReadResource(context.Background(), &runtime.ReadResourceRequest{
		TypeName:     "example_resource",
		CurrentState: initial,
	})
	require.NoError(t, err)
	assertStateEnvelope(t, read.NewState, `{"id":"read","name":"before"}`, []byte("private-read"), 2)

	planned, err := adapter.PlanResourceChange(context.Background(), &runtime.PlanResourceChangeRequest{
		TypeName:      "example_resource",
		PriorState:    read.NewState,
		ProposedState: runtime.DynamicValue{JSON: []byte(`{"id":"read","name":"after"}`)},
		Config:        runtime.DynamicValue{JSON: []byte(`{"id":null,"name":"after"}`)},
		PriorPrivate:  []byte("-extra"),
	})
	require.NoError(t, err)
	assertStateEnvelope(t, planned.PlannedState, `{"id":"planned","name":"after"}`, []byte("private-plan"), 2)
	assert.Equal(t, []string{"name"}, planned.ReplacementMetadata.RequiresReplace)

	applied, err := adapter.ApplyResourceChange(context.Background(), &runtime.ApplyResourceChangeRequest{
		TypeName:       "example_resource",
		PriorState:     read.NewState,
		PlannedState:   planned.PlannedState,
		Config:         runtime.DynamicValue{JSON: []byte(`{"id":null,"name":"after"}`)},
		PlannedPrivate: []byte("-extra"),
	})
	require.NoError(t, err)
	assertStateEnvelope(t, applied.NewState, `{"id":"applied","name":"after"}`, []byte("private-apply"), 2)
}

func TestV5AdapterActions(t *testing.T) {
	adapter := NewV5Adapter(newFakeV5Provider())
	_, err := adapter.Schema(context.Background())
	require.NoError(t, err)

	err = adapter.ValidateAction(context.Background(), &runtime.ValidateActionRequest{
		TypeName: "example_action",
		Config:   runtime.DynamicValue{JSON: []byte(`{"name":"run"}`)},
	})
	require.NoError(t, err)

	plan, err := adapter.PlanAction(context.Background(), &runtime.PlanActionRequest{
		TypeName: "example_action",
		Config:   runtime.DynamicValue{JSON: []byte(`{"name":"run"}`)},
	})
	require.NoError(t, err)
	assert.False(t, plan.Deferred)

	invoked, err := adapter.InvokeAction(context.Background(), &runtime.InvokeActionRequest{
		TypeName: "example_action",
		Config:   runtime.DynamicValue{JSON: []byte(`{"name":"run"}`)},
	})
	require.NoError(t, err)
	require.Len(t, invoked.Events, 2)
	assert.Equal(t, "progress", invoked.Events[0].Type)
	assert.Equal(t, "halfway", invoked.Events[0].Message)
	assert.Equal(t, "completed", invoked.Events[1].Type)
}

func TestV5AdapterDiagnosticsBecomeErrors(t *testing.T) {
	provider := newFakeV5Provider()
	provider.validateResourceDiagnostics = []*tfprotov5.Diagnostic{{
		Severity: tfprotov5.DiagnosticSeverityError,
		Summary:  "invalid resource",
	}}
	adapter := NewV5Adapter(provider)
	_, err := adapter.Schema(context.Background())
	require.NoError(t, err)

	err = adapter.ValidateResourceConfig(context.Background(), &runtime.ValidateResourceConfigRequest{
		TypeName: "example_resource",
		Config:   runtime.DynamicValue{JSON: []byte(`{"id":null,"name":"bad"}`)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource")
}

type fakeV5Provider struct {
	configured                  runtime.DynamicValue
	validateResourceDiagnostics []*tfprotov5.Diagnostic
	schemaCalls                 int
	identitySchemasErr          error
}

func newFakeV5Provider() *fakeV5Provider {
	return &fakeV5Provider{}
}

func (p *fakeV5Provider) GetMetadata(context.Context, *tfprotov5.GetMetadataRequest) (*tfprotov5.GetMetadataResponse, error) {
	return &tfprotov5.GetMetadataResponse{}, nil
}

func (p *fakeV5Provider) GetProviderSchema(context.Context, *tfprotov5.GetProviderSchemaRequest) (*tfprotov5.GetProviderSchemaResponse, error) {
	p.schemaCalls++
	return &tfprotov5.GetProviderSchemaResponse{
		Provider:          providerSchema(),
		ResourceSchemas:   map[string]*tfprotov5.Schema{"example_resource": resourceSchema()},
		DataSourceSchemas: map[string]*tfprotov5.Schema{"example_data": namedSchema()},
		ActionSchemas:     map[string]*tfprotov5.ActionSchema{"example_action": {Schema: namedSchema()}},
	}, nil
}

func (p *fakeV5Provider) GetResourceIdentitySchemas(context.Context, *tfprotov5.GetResourceIdentitySchemasRequest) (*tfprotov5.GetResourceIdentitySchemasResponse, error) {
	if p.identitySchemasErr != nil {
		return nil, p.identitySchemasErr
	}
	return &tfprotov5.GetResourceIdentitySchemasResponse{}, nil
}

func (p *fakeV5Provider) PrepareProviderConfig(_ context.Context, req *tfprotov5.PrepareProviderConfigRequest) (*tfprotov5.PrepareProviderConfigResponse, error) {
	prepared := mustDynamic(providerSchema().ValueType(), map[string]tftypes.Value{"token": tftypes.NewValue(tftypes.String, "prepared")})
	return &tfprotov5.PrepareProviderConfigResponse{PreparedConfig: prepared}, nil
}

func (p *fakeV5Provider) ConfigureProvider(_ context.Context, req *tfprotov5.ConfigureProviderRequest) (*tfprotov5.ConfigureProviderResponse, error) {
	raw, _ := DynamicValueToJSON(providerSchema().ValueType(), req.Config)
	p.configured = runtime.DynamicValue{JSON: raw}
	return &tfprotov5.ConfigureProviderResponse{}, nil
}

func (p *fakeV5Provider) StopProvider(context.Context, *tfprotov5.StopProviderRequest) (*tfprotov5.StopProviderResponse, error) {
	return &tfprotov5.StopProviderResponse{}, nil
}

func (p *fakeV5Provider) ValidateResourceTypeConfig(context.Context, *tfprotov5.ValidateResourceTypeConfigRequest) (*tfprotov5.ValidateResourceTypeConfigResponse, error) {
	return &tfprotov5.ValidateResourceTypeConfigResponse{Diagnostics: p.validateResourceDiagnostics}, nil
}

func (p *fakeV5Provider) UpgradeResourceState(context.Context, *tfprotov5.UpgradeResourceStateRequest) (*tfprotov5.UpgradeResourceStateResponse, error) {
	return &tfprotov5.UpgradeResourceStateResponse{}, nil
}

func (p *fakeV5Provider) ReadResource(_ context.Context, req *tfprotov5.ReadResourceRequest) (*tfprotov5.ReadResourceResponse, error) {
	current, _ := req.CurrentState.Unmarshal(resourceSchema().ValueType())
	values := map[string]tftypes.Value{}
	_ = current.As(&values)
	newState := mustDynamic(resourceSchema().ValueType(), map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "read"),
		"name": values["name"],
	})
	return &tfprotov5.ReadResourceResponse{NewState: newState, Private: []byte("private-read")}, nil
}

func (p *fakeV5Provider) PlanResourceChange(context.Context, *tfprotov5.PlanResourceChangeRequest) (*tfprotov5.PlanResourceChangeResponse, error) {
	planned := mustDynamic(resourceSchema().ValueType(), map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "planned"),
		"name": tftypes.NewValue(tftypes.String, "after"),
	})
	return &tfprotov5.PlanResourceChangeResponse{
		PlannedState:    planned,
		PlannedPrivate:  []byte("private-plan"),
		RequiresReplace: []*tftypes.AttributePath{tftypes.NewAttributePath().WithAttributeName("name")},
	}, nil
}

func (p *fakeV5Provider) ApplyResourceChange(context.Context, *tfprotov5.ApplyResourceChangeRequest) (*tfprotov5.ApplyResourceChangeResponse, error) {
	applied := mustDynamic(resourceSchema().ValueType(), map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "applied"),
		"name": tftypes.NewValue(tftypes.String, "after"),
	})
	return &tfprotov5.ApplyResourceChangeResponse{NewState: applied, Private: []byte("private-apply")}, nil
}

func (p *fakeV5Provider) ImportResourceState(context.Context, *tfprotov5.ImportResourceStateRequest) (*tfprotov5.ImportResourceStateResponse, error) {
	return &tfprotov5.ImportResourceStateResponse{}, nil
}

func (p *fakeV5Provider) MoveResourceState(context.Context, *tfprotov5.MoveResourceStateRequest) (*tfprotov5.MoveResourceStateResponse, error) {
	return &tfprotov5.MoveResourceStateResponse{}, nil
}

func (p *fakeV5Provider) UpgradeResourceIdentity(context.Context, *tfprotov5.UpgradeResourceIdentityRequest) (*tfprotov5.UpgradeResourceIdentityResponse, error) {
	return &tfprotov5.UpgradeResourceIdentityResponse{}, nil
}

func (p *fakeV5Provider) GenerateResourceConfig(context.Context, *tfprotov5.GenerateResourceConfigRequest) (*tfprotov5.GenerateResourceConfigResponse, error) {
	return &tfprotov5.GenerateResourceConfigResponse{}, nil
}

func (p *fakeV5Provider) ValidateDataSourceConfig(context.Context, *tfprotov5.ValidateDataSourceConfigRequest) (*tfprotov5.ValidateDataSourceConfigResponse, error) {
	return &tfprotov5.ValidateDataSourceConfigResponse{}, nil
}

func (p *fakeV5Provider) ReadDataSource(context.Context, *tfprotov5.ReadDataSourceRequest) (*tfprotov5.ReadDataSourceResponse, error) {
	return &tfprotov5.ReadDataSourceResponse{
		State: mustDynamic(namedSchema().ValueType(), map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "from-data")}),
	}, nil
}

func (p *fakeV5Provider) GetFunctions(context.Context, *tfprotov5.GetFunctionsRequest) (*tfprotov5.GetFunctionsResponse, error) {
	return &tfprotov5.GetFunctionsResponse{}, nil
}

func (p *fakeV5Provider) CallFunction(context.Context, *tfprotov5.CallFunctionRequest) (*tfprotov5.CallFunctionResponse, error) {
	return &tfprotov5.CallFunctionResponse{}, nil
}

func (p *fakeV5Provider) ValidateEphemeralResourceConfig(context.Context, *tfprotov5.ValidateEphemeralResourceConfigRequest) (*tfprotov5.ValidateEphemeralResourceConfigResponse, error) {
	return &tfprotov5.ValidateEphemeralResourceConfigResponse{}, nil
}

func (p *fakeV5Provider) OpenEphemeralResource(context.Context, *tfprotov5.OpenEphemeralResourceRequest) (*tfprotov5.OpenEphemeralResourceResponse, error) {
	return &tfprotov5.OpenEphemeralResourceResponse{}, nil
}

func (p *fakeV5Provider) RenewEphemeralResource(context.Context, *tfprotov5.RenewEphemeralResourceRequest) (*tfprotov5.RenewEphemeralResourceResponse, error) {
	return &tfprotov5.RenewEphemeralResourceResponse{}, nil
}

func (p *fakeV5Provider) CloseEphemeralResource(context.Context, *tfprotov5.CloseEphemeralResourceRequest) (*tfprotov5.CloseEphemeralResourceResponse, error) {
	return &tfprotov5.CloseEphemeralResourceResponse{}, nil
}

func (p *fakeV5Provider) ValidateActionConfig(context.Context, *tfprotov5.ValidateActionConfigRequest) (*tfprotov5.ValidateActionConfigResponse, error) {
	return &tfprotov5.ValidateActionConfigResponse{}, nil
}

func (p *fakeV5Provider) PlanAction(context.Context, *tfprotov5.PlanActionRequest) (*tfprotov5.PlanActionResponse, error) {
	return &tfprotov5.PlanActionResponse{}, nil
}

func (p *fakeV5Provider) InvokeAction(context.Context, *tfprotov5.InvokeActionRequest) (*tfprotov5.InvokeActionServerStream, error) {
	return &tfprotov5.InvokeActionServerStream{
		Events: func(yield func(tfprotov5.InvokeActionEvent) bool) {
			if !yield(tfprotov5.InvokeActionEvent{Type: tfprotov5.ProgressInvokeActionEventType{Message: "halfway"}}) {
				return
			}
			yield(tfprotov5.InvokeActionEvent{Type: tfprotov5.CompletedInvokeActionEventType{}})
		},
	}, nil
}

func providerSchema() *tfprotov5.Schema {
	return &tfprotov5.Schema{Block: &tfprotov5.SchemaBlock{Attributes: []*tfprotov5.SchemaAttribute{
		{Name: "token", Type: tftypes.String, Required: true},
	}}}
}

func namedSchema() *tfprotov5.Schema {
	return &tfprotov5.Schema{Version: 1, Block: &tfprotov5.SchemaBlock{Attributes: []*tfprotov5.SchemaAttribute{
		{Name: "name", Type: tftypes.String, Required: true},
	}}}
}

func resourceSchema() *tfprotov5.Schema {
	return &tfprotov5.Schema{Version: 2, Block: &tfprotov5.SchemaBlock{Attributes: []*tfprotov5.SchemaAttribute{
		{Name: "id", Type: tftypes.String, Computed: true},
		{Name: "name", Type: tftypes.String, Required: true},
	}}}
}

func mustDynamic(typ tftypes.Type, values map[string]tftypes.Value) *tfprotov5.DynamicValue {
	dynamic, err := tfprotov5.NewDynamicValue(typ, tftypes.NewValue(typ, values))
	if err != nil {
		panic(err)
	}
	return &dynamic
}

func providerState(t *testing.T, typeName, value string, private []byte, version int64) runtime.ProviderState {
	t.Helper()
	raw, err := json.Marshal(runtime.StateEnvelope{
		FormatVersion: 1,
		Protocol:      protocolName,
		TypeName:      typeName,
		SchemaVersion: version,
		Value:         runtime.DynamicValue{JSON: []byte(value)},
		Private:       private,
	})
	require.NoError(t, err)
	return runtime.ProviderState{Envelope: raw}
}

func assertStateEnvelope(t *testing.T, state runtime.ProviderState, wantValue string, wantPrivate []byte, wantVersion int64) {
	t.Helper()
	var envelope runtime.StateEnvelope
	require.NoError(t, json.Unmarshal(state.Envelope, &envelope))
	assert.Equal(t, protocolName, envelope.Protocol)
	assert.Equal(t, wantVersion, envelope.SchemaVersion)
	assert.Equal(t, wantPrivate, envelope.Private)
	assert.JSONEq(t, wantValue, string(envelope.Value.JSON))
}

var _ iter.Seq[tfprotov5.InvokeActionEvent]
