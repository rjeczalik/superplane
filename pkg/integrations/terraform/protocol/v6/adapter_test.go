package v6

import (
	"context"
	"encoding/json"
	"iter"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestV6AdapterSchema(t *testing.T) {
	adapter := NewV6Adapter(newFakeV6Provider())

	schema, err := adapter.Schema(context.Background())
	require.NoError(t, err)

	assert.NotEmpty(t, schema.Provider.Block)
	assert.Equal(t, int64(2), schema.Resources["example_resource"].Version)
	assert.Contains(t, string(schema.Resources["example_resource"].Block), `"id"`)
	assert.Contains(t, string(schema.DataSources["example_data"].Block), `"name"`)
	assert.Contains(t, string(schema.Actions["example_action"].Block), `"name"`)
}

func TestV6AdapterSchemaToleratesUnimplementedResourceIdentitySchemas(t *testing.T) {
	provider := newFakeV6Provider()
	provider.identitySchemasErr = status.Error(codes.Unimplemented, "unknown method GetResourceIdentitySchemas")
	adapter := NewV6Adapter(provider)

	schema, err := adapter.Schema(context.Background())
	require.NoError(t, err)
	assert.Contains(t, schema.Resources, "example_resource")
}

func TestV6AdapterConfigMethods(t *testing.T) {
	provider := newFakeV6Provider()
	adapter := NewV6Adapter(provider)
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

func TestV6AdapterConfigMethodsHydrateSchemaLazily(t *testing.T) {
	provider := newFakeV6Provider()
	adapter := NewV6Adapter(provider)

	err := adapter.Configure(context.Background(), &runtime.ConfigureRequest{Config: runtime.DynamicValue{JSON: []byte(`{"token":"abc"}`)}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"token":"abc"}`, string(provider.configured.JSON))
	assert.Equal(t, 1, provider.schemaCalls)
}

func TestV6AdapterDataSource(t *testing.T) {
	adapter := NewV6Adapter(newFakeV6Provider())
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

func TestV6AdapterResourceLifecycle(t *testing.T) {
	adapter := NewV6Adapter(newFakeV6Provider())
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

func TestV6AdapterActions(t *testing.T) {
	adapter := NewV6Adapter(newFakeV6Provider())
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

func TestV6AdapterDiagnosticsBecomeErrors(t *testing.T) {
	provider := newFakeV6Provider()
	provider.validateResourceDiagnostics = []*tfprotov6.Diagnostic{{
		Severity: tfprotov6.DiagnosticSeverityError,
		Summary:  "invalid resource",
	}}
	adapter := NewV6Adapter(provider)
	_, err := adapter.Schema(context.Background())
	require.NoError(t, err)

	err = adapter.ValidateResourceConfig(context.Background(), &runtime.ValidateResourceConfigRequest{
		TypeName: "example_resource",
		Config:   runtime.DynamicValue{JSON: []byte(`{"id":null,"name":"bad"}`)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource")
}

type fakeV6Provider struct {
	configured                  runtime.DynamicValue
	validateResourceDiagnostics []*tfprotov6.Diagnostic
	schemaCalls                 int
	identitySchemasErr          error
}

func newFakeV6Provider() *fakeV6Provider {
	return &fakeV6Provider{}
}

func (p *fakeV6Provider) GetMetadata(context.Context, *tfprotov6.GetMetadataRequest) (*tfprotov6.GetMetadataResponse, error) {
	return &tfprotov6.GetMetadataResponse{}, nil
}

func (p *fakeV6Provider) GetProviderSchema(context.Context, *tfprotov6.GetProviderSchemaRequest) (*tfprotov6.GetProviderSchemaResponse, error) {
	p.schemaCalls++
	return &tfprotov6.GetProviderSchemaResponse{
		Provider:          providerSchema(),
		ResourceSchemas:   map[string]*tfprotov6.Schema{"example_resource": resourceSchema()},
		DataSourceSchemas: map[string]*tfprotov6.Schema{"example_data": namedSchema()},
		ActionSchemas:     map[string]*tfprotov6.ActionSchema{"example_action": {Schema: namedSchema()}},
	}, nil
}

func (p *fakeV6Provider) GetResourceIdentitySchemas(context.Context, *tfprotov6.GetResourceIdentitySchemasRequest) (*tfprotov6.GetResourceIdentitySchemasResponse, error) {
	if p.identitySchemasErr != nil {
		return nil, p.identitySchemasErr
	}
	return &tfprotov6.GetResourceIdentitySchemasResponse{}, nil
}

func (p *fakeV6Provider) ValidateProviderConfig(_ context.Context, req *tfprotov6.ValidateProviderConfigRequest) (*tfprotov6.ValidateProviderConfigResponse, error) {
	prepared := mustDynamic(providerSchema().ValueType(), map[string]tftypes.Value{"token": tftypes.NewValue(tftypes.String, "prepared")})
	return &tfprotov6.ValidateProviderConfigResponse{PreparedConfig: prepared}, nil
}

func (p *fakeV6Provider) ConfigureProvider(_ context.Context, req *tfprotov6.ConfigureProviderRequest) (*tfprotov6.ConfigureProviderResponse, error) {
	raw, _ := DynamicValueToJSON(providerSchema().ValueType(), req.Config)
	p.configured = runtime.DynamicValue{JSON: raw}
	return &tfprotov6.ConfigureProviderResponse{}, nil
}

func (p *fakeV6Provider) StopProvider(context.Context, *tfprotov6.StopProviderRequest) (*tfprotov6.StopProviderResponse, error) {
	return &tfprotov6.StopProviderResponse{}, nil
}

func (p *fakeV6Provider) ValidateResourceConfig(context.Context, *tfprotov6.ValidateResourceConfigRequest) (*tfprotov6.ValidateResourceConfigResponse, error) {
	return &tfprotov6.ValidateResourceConfigResponse{Diagnostics: p.validateResourceDiagnostics}, nil
}

func (p *fakeV6Provider) UpgradeResourceState(context.Context, *tfprotov6.UpgradeResourceStateRequest) (*tfprotov6.UpgradeResourceStateResponse, error) {
	return &tfprotov6.UpgradeResourceStateResponse{}, nil
}

func (p *fakeV6Provider) ReadResource(_ context.Context, req *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	current, _ := req.CurrentState.Unmarshal(resourceSchema().ValueType())
	values := map[string]tftypes.Value{}
	_ = current.As(&values)
	newState := mustDynamic(resourceSchema().ValueType(), map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "read"),
		"name": values["name"],
	})
	return &tfprotov6.ReadResourceResponse{NewState: newState, Private: []byte("private-read")}, nil
}

func (p *fakeV6Provider) PlanResourceChange(context.Context, *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	planned := mustDynamic(resourceSchema().ValueType(), map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "planned"),
		"name": tftypes.NewValue(tftypes.String, "after"),
	})
	return &tfprotov6.PlanResourceChangeResponse{
		PlannedState:    planned,
		PlannedPrivate:  []byte("private-plan"),
		RequiresReplace: []*tftypes.AttributePath{tftypes.NewAttributePath().WithAttributeName("name")},
	}, nil
}

func (p *fakeV6Provider) ApplyResourceChange(context.Context, *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	applied := mustDynamic(resourceSchema().ValueType(), map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "applied"),
		"name": tftypes.NewValue(tftypes.String, "after"),
	})
	return &tfprotov6.ApplyResourceChangeResponse{NewState: applied, Private: []byte("private-apply")}, nil
}

func (p *fakeV6Provider) ImportResourceState(context.Context, *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error) {
	return &tfprotov6.ImportResourceStateResponse{}, nil
}

func (p *fakeV6Provider) MoveResourceState(context.Context, *tfprotov6.MoveResourceStateRequest) (*tfprotov6.MoveResourceStateResponse, error) {
	return &tfprotov6.MoveResourceStateResponse{}, nil
}

func (p *fakeV6Provider) UpgradeResourceIdentity(context.Context, *tfprotov6.UpgradeResourceIdentityRequest) (*tfprotov6.UpgradeResourceIdentityResponse, error) {
	return &tfprotov6.UpgradeResourceIdentityResponse{}, nil
}

func (p *fakeV6Provider) GenerateResourceConfig(context.Context, *tfprotov6.GenerateResourceConfigRequest) (*tfprotov6.GenerateResourceConfigResponse, error) {
	return &tfprotov6.GenerateResourceConfigResponse{}, nil
}

func (p *fakeV6Provider) ValidateDataResourceConfig(context.Context, *tfprotov6.ValidateDataResourceConfigRequest) (*tfprotov6.ValidateDataResourceConfigResponse, error) {
	return &tfprotov6.ValidateDataResourceConfigResponse{}, nil
}

func (p *fakeV6Provider) ReadDataSource(context.Context, *tfprotov6.ReadDataSourceRequest) (*tfprotov6.ReadDataSourceResponse, error) {
	return &tfprotov6.ReadDataSourceResponse{
		State: mustDynamic(namedSchema().ValueType(), map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "from-data")}),
	}, nil
}

func (p *fakeV6Provider) GetFunctions(context.Context, *tfprotov6.GetFunctionsRequest) (*tfprotov6.GetFunctionsResponse, error) {
	return &tfprotov6.GetFunctionsResponse{}, nil
}

func (p *fakeV6Provider) CallFunction(context.Context, *tfprotov6.CallFunctionRequest) (*tfprotov6.CallFunctionResponse, error) {
	return &tfprotov6.CallFunctionResponse{}, nil
}

func (p *fakeV6Provider) ValidateEphemeralResourceConfig(context.Context, *tfprotov6.ValidateEphemeralResourceConfigRequest) (*tfprotov6.ValidateEphemeralResourceConfigResponse, error) {
	return &tfprotov6.ValidateEphemeralResourceConfigResponse{}, nil
}

func (p *fakeV6Provider) OpenEphemeralResource(context.Context, *tfprotov6.OpenEphemeralResourceRequest) (*tfprotov6.OpenEphemeralResourceResponse, error) {
	return &tfprotov6.OpenEphemeralResourceResponse{}, nil
}

func (p *fakeV6Provider) RenewEphemeralResource(context.Context, *tfprotov6.RenewEphemeralResourceRequest) (*tfprotov6.RenewEphemeralResourceResponse, error) {
	return &tfprotov6.RenewEphemeralResourceResponse{}, nil
}

func (p *fakeV6Provider) CloseEphemeralResource(context.Context, *tfprotov6.CloseEphemeralResourceRequest) (*tfprotov6.CloseEphemeralResourceResponse, error) {
	return &tfprotov6.CloseEphemeralResourceResponse{}, nil
}

func (p *fakeV6Provider) ValidateActionConfig(context.Context, *tfprotov6.ValidateActionConfigRequest) (*tfprotov6.ValidateActionConfigResponse, error) {
	return &tfprotov6.ValidateActionConfigResponse{}, nil
}

func (p *fakeV6Provider) PlanAction(context.Context, *tfprotov6.PlanActionRequest) (*tfprotov6.PlanActionResponse, error) {
	return &tfprotov6.PlanActionResponse{}, nil
}

func (p *fakeV6Provider) InvokeAction(context.Context, *tfprotov6.InvokeActionRequest) (*tfprotov6.InvokeActionServerStream, error) {
	return &tfprotov6.InvokeActionServerStream{
		Events: func(yield func(tfprotov6.InvokeActionEvent) bool) {
			if !yield(tfprotov6.InvokeActionEvent{Type: tfprotov6.ProgressInvokeActionEventType{Message: "halfway"}}) {
				return
			}
			yield(tfprotov6.InvokeActionEvent{Type: tfprotov6.CompletedInvokeActionEventType{}})
		},
	}, nil
}

func providerSchema() *tfprotov6.Schema {
	return &tfprotov6.Schema{Block: &tfprotov6.SchemaBlock{Attributes: []*tfprotov6.SchemaAttribute{
		{Name: "token", Type: tftypes.String, Required: true},
	}}}
}

func namedSchema() *tfprotov6.Schema {
	return &tfprotov6.Schema{Version: 1, Block: &tfprotov6.SchemaBlock{Attributes: []*tfprotov6.SchemaAttribute{
		{Name: "name", Type: tftypes.String, Required: true},
	}}}
}

func resourceSchema() *tfprotov6.Schema {
	return &tfprotov6.Schema{Version: 2, Block: &tfprotov6.SchemaBlock{Attributes: []*tfprotov6.SchemaAttribute{
		{Name: "id", Type: tftypes.String, Computed: true},
		{Name: "name", Type: tftypes.String, Required: true},
	}}}
}

func mustDynamic(typ tftypes.Type, values map[string]tftypes.Value) *tfprotov6.DynamicValue {
	dynamic, err := tfprotov6.NewDynamicValue(typ, tftypes.NewValue(typ, values))
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

var _ iter.Seq[tfprotov6.InvokeActionEvent]
