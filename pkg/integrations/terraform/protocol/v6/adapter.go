package v6

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

const protocolName = "tfprotov6"
const closeTimeout = 5 * time.Second

type V6Adapter struct {
	client tfprotov6.ProviderServer
	schema *schemaCache
}

func NewV6Adapter(client tfprotov6.ProviderServer) *V6Adapter {
	return &V6Adapter{client: client}
}

func (a *V6Adapter) Schema(ctx context.Context) (*runtime.SchemaResponse, error) {
	resp, err := a.client.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		return nil, err
	}
	identityResp, err := a.client.GetResourceIdentitySchemas(ctx, &tfprotov6.GetResourceIdentitySchemasRequest{})
	if err != nil {
		if status.Code(err) != codes.Unimplemented {
			return nil, err
		}
		identityResp = &tfprotov6.GetResourceIdentitySchemasResponse{}
	}

	a.schema = newSchemaCache(resp, identityResp)
	out := &runtime.SchemaResponse{
		Provider:    runtime.ProviderSchemaBlock{Block: marshalSchemaBlock(resp.Provider)},
		Resources:   make(map[string]runtime.SchemaBlock, len(resp.ResourceSchemas)),
		DataSources: make(map[string]runtime.SchemaBlock, len(resp.DataSourceSchemas)),
		Actions:     make(map[string]runtime.SchemaBlock, len(resp.ActionSchemas)),
	}
	for name, schema := range resp.ResourceSchemas {
		out.Resources[name] = runtime.SchemaBlock{Version: schemaVersion(schema), Block: marshalSchemaBlock(schema)}
	}
	for name, schema := range resp.DataSourceSchemas {
		out.DataSources[name] = runtime.SchemaBlock{Version: schemaVersion(schema), Block: marshalSchemaBlock(schema)}
	}
	for name, schema := range resp.ActionSchemas {
		out.Actions[name] = runtime.SchemaBlock{Version: schemaVersion(schema.Schema), Block: marshalSchemaBlock(schema.Schema)}
	}

	return out, diagnosticsError(append(resp.Diagnostics, identityResp.Diagnostics...))
}

func (a *V6Adapter) PrepareProviderConfig(ctx context.Context, req *runtime.PrepareProviderConfigRequest) (*runtime.PrepareProviderConfigResponse, error) {
	if err := a.ensureSchema(ctx); err != nil {
		return nil, err
	}
	config, err := a.dynamicFromJSON(a.providerType(), req.Config)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.ValidateProviderConfig(ctx, &tfprotov6.ValidateProviderConfigRequest{Config: config})
	if err != nil {
		return nil, err
	}

	prepared, err := a.dynamicToJSON(a.providerType(), resp.PreparedConfig)
	if err != nil {
		return nil, err
	}

	return &runtime.PrepareProviderConfigResponse{
		PreparedConfig: prepared,
		Diagnostics:    DiagnosticsFromTFProto(resp.Diagnostics),
	}, diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) ValidateProviderConfig(ctx context.Context, req *runtime.ValidateProviderConfigRequest) error {
	if err := a.ensureSchema(ctx); err != nil {
		return err
	}
	config, err := a.dynamicFromJSON(a.providerType(), req.Config)
	if err != nil {
		return err
	}

	resp, err := a.client.ValidateProviderConfig(ctx, &tfprotov6.ValidateProviderConfigRequest{Config: config})
	if err != nil {
		return err
	}
	return diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) Configure(ctx context.Context, req *runtime.ConfigureRequest) error {
	if err := a.ensureSchema(ctx); err != nil {
		return err
	}
	config, err := a.dynamicFromJSON(a.providerType(), req.Config)
	if err != nil {
		return err
	}

	resp, err := a.client.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{Config: config})
	if err != nil {
		return err
	}
	return diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) ValidateDataSourceConfig(ctx context.Context, req *runtime.ValidateDataSourceConfigRequest) error {
	if err := a.ensureSchema(ctx); err != nil {
		return err
	}
	config, err := a.dynamicFromJSON(a.dataSourceType(req.TypeName), req.Config)
	if err != nil {
		return err
	}

	resp, err := a.client.ValidateDataResourceConfig(ctx, &tfprotov6.ValidateDataResourceConfigRequest{TypeName: req.TypeName, Config: config})
	if err != nil {
		return err
	}
	return diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) ReadDataSource(ctx context.Context, req *runtime.ReadDataSourceRequest) (*runtime.ReadDataSourceResponse, error) {
	if err := a.ensureSchema(ctx); err != nil {
		return nil, err
	}
	typ := a.dataSourceType(req.TypeName)
	config, err := a.dynamicFromJSON(typ, req.Config)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.ReadDataSource(ctx, &tfprotov6.ReadDataSourceRequest{TypeName: req.TypeName, Config: config})
	if err != nil {
		return nil, err
	}

	state, err := a.dynamicToJSON(typ, resp.State)
	if err != nil {
		return nil, err
	}
	return &runtime.ReadDataSourceResponse{State: state, Diagnostics: DiagnosticsFromTFProto(resp.Diagnostics)}, diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) ValidateAction(ctx context.Context, req *runtime.ValidateActionRequest) error {
	if err := a.ensureSchema(ctx); err != nil {
		return err
	}
	actions, ok := a.client.(tfprotov6.ProviderServerWithActions)
	if !ok {
		return fmt.Errorf("provider does not implement v6 actions")
	}
	config, err := a.dynamicFromJSON(a.actionType(req.TypeName), req.Config)
	if err != nil {
		return err
	}

	resp, err := actions.ValidateActionConfig(ctx, &tfprotov6.ValidateActionConfigRequest{ActionType: req.TypeName, Config: config})
	if err != nil {
		return err
	}
	return diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) PlanAction(ctx context.Context, req *runtime.PlanActionRequest) (*runtime.PlanActionResponse, error) {
	if err := a.ensureSchema(ctx); err != nil {
		return nil, err
	}
	actions, ok := a.client.(tfprotov6.ProviderServerWithActions)
	if !ok {
		return nil, fmt.Errorf("provider does not implement v6 actions")
	}
	config, err := a.dynamicFromJSON(a.actionType(req.TypeName), req.Config)
	if err != nil {
		return nil, err
	}

	resp, err := actions.PlanAction(ctx, &tfprotov6.PlanActionRequest{ActionType: req.TypeName, Config: config})
	if err != nil {
		return nil, err
	}

	return &runtime.PlanActionResponse{
		Diagnostics: DiagnosticsFromTFProto(resp.Diagnostics),
		Deferred:    resp.Deferred != nil,
	}, diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) InvokeAction(ctx context.Context, req *runtime.InvokeActionRequest) (*runtime.InvokeActionResponse, error) {
	if err := a.ensureSchema(ctx); err != nil {
		return nil, err
	}
	actions, ok := a.client.(tfprotov6.ProviderServerWithActions)
	if !ok {
		return nil, fmt.Errorf("provider does not implement v6 actions")
	}
	config, err := a.dynamicFromJSON(a.actionType(req.TypeName), req.Config)
	if err != nil {
		return nil, err
	}

	stream, err := actions.InvokeAction(ctx, &tfprotov6.InvokeActionRequest{ActionType: req.TypeName, Config: config})
	if err != nil {
		return nil, err
	}

	out := &runtime.InvokeActionResponse{}
	if stream == nil || stream.Events == nil {
		return out, nil
	}
	for event := range stream.Events {
		translated := runtime.ActionEvent{}
		switch typ := event.Type.(type) {
		case tfprotov6.ProgressInvokeActionEventType:
			translated.Type = "progress"
			translated.Message = typ.Message
		case tfprotov6.CompletedInvokeActionEventType:
			translated.Type = "completed"
			translated.Diagnostics = DiagnosticsFromTFProto(typ.Diagnostics)
			out.Diagnostics = append(out.Diagnostics, translated.Diagnostics...)
		default:
			translated.Type = "unknown"
		}
		out.Events = append(out.Events, translated)
	}
	return out, diagnosticsError(toTFDiagnostics(out.Diagnostics))
}

func (a *V6Adapter) ValidateResourceConfig(ctx context.Context, req *runtime.ValidateResourceConfigRequest) error {
	if err := a.ensureSchema(ctx); err != nil {
		return err
	}
	config, err := a.dynamicFromJSON(a.resourceType(req.TypeName), req.Config)
	if err != nil {
		return err
	}

	resp, err := a.client.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{TypeName: req.TypeName, Config: config})
	if err != nil {
		return err
	}
	return diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) ReadResource(ctx context.Context, req *runtime.ReadResourceRequest) (*runtime.ReadResourceResponse, error) {
	if err := a.ensureSchema(ctx); err != nil {
		return nil, err
	}
	typ := a.resourceType(req.TypeName)
	current, private, identity, _, err := providerStateParts(req.CurrentState)
	if err != nil {
		return nil, err
	}
	currentState, err := a.dynamicFromJSON(typ, runtime.DynamicValue{JSON: current})
	if err != nil {
		return nil, err
	}

	currentIdentity, err := a.identityFromJSON(req.TypeName, identity)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: req.TypeName, CurrentState: currentState, Private: private, CurrentIdentity: currentIdentity})
	if err != nil {
		return nil, err
	}
	state, err := a.stateFromDynamic(req.TypeName, resp.NewState, resp.Private, resp.NewIdentity)
	if err != nil {
		return nil, err
	}
	return &runtime.ReadResourceResponse{NewState: state, Diagnostics: DiagnosticsFromTFProto(resp.Diagnostics)}, diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) PlanResourceChange(ctx context.Context, req *runtime.PlanResourceChangeRequest) (*runtime.PlanResourceChangeResponse, error) {
	if err := a.ensureSchema(ctx); err != nil {
		return nil, err
	}
	typ := a.resourceType(req.TypeName)
	priorJSON, priorPrivate, priorIdentity, _, err := providerStateParts(req.PriorState)
	if err != nil {
		return nil, err
	}
	priorState, err := a.dynamicFromJSON(typ, runtime.DynamicValue{JSON: priorJSON})
	if err != nil {
		return nil, err
	}
	proposed, err := a.dynamicFromJSON(typ, req.ProposedState)
	if err != nil {
		return nil, err
	}
	config, err := a.dynamicFromJSON(typ, req.Config)
	if err != nil {
		return nil, err
	}
	identity, err := a.identityFromJSON(req.TypeName, priorIdentity)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         req.TypeName,
		PriorState:       priorState,
		ProposedNewState: proposed,
		Config:           config,
		PriorPrivate:     privateOrFallback(priorPrivate, req.PriorPrivate),
		PriorIdentity:    identity,
	})
	if err != nil {
		return nil, err
	}

	state, err := a.stateFromDynamic(req.TypeName, resp.PlannedState, resp.PlannedPrivate, resp.PlannedIdentity)
	if err != nil {
		return nil, err
	}
	return &runtime.PlanResourceChangeResponse{
		PlannedState:        state,
		ReplacementMetadata: runtime.ReplacementMetadata{RequiresReplace: formatAttributePaths(resp.RequiresReplace)},
		Diagnostics:         DiagnosticsFromTFProto(resp.Diagnostics),
	}, diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) ApplyResourceChange(ctx context.Context, req *runtime.ApplyResourceChangeRequest) (*runtime.ApplyResourceChangeResponse, error) {
	if err := a.ensureSchema(ctx); err != nil {
		return nil, err
	}
	typ := a.resourceType(req.TypeName)
	priorJSON, _, _, _, err := providerStateParts(req.PriorState)
	if err != nil {
		return nil, err
	}
	plannedJSON, plannedPrivate, plannedIdentity, _, err := providerStateParts(req.PlannedState)
	if err != nil {
		return nil, err
	}
	priorState, err := a.dynamicFromJSON(typ, runtime.DynamicValue{JSON: priorJSON})
	if err != nil {
		return nil, err
	}
	plannedState, err := a.dynamicFromJSON(typ, runtime.DynamicValue{JSON: plannedJSON})
	if err != nil {
		return nil, err
	}
	config, err := a.dynamicFromJSON(typ, req.Config)
	if err != nil {
		return nil, err
	}
	identity, err := a.identityFromJSON(req.TypeName, plannedIdentity)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{
		TypeName:        req.TypeName,
		PriorState:      priorState,
		PlannedState:    plannedState,
		Config:          config,
		PlannedPrivate:  privateOrFallback(plannedPrivate, req.PlannedPrivate),
		PlannedIdentity: identity,
	})
	if err != nil {
		return nil, err
	}

	state, err := a.stateFromDynamic(req.TypeName, resp.NewState, resp.Private, resp.NewIdentity)
	if err != nil {
		return nil, err
	}
	return &runtime.ApplyResourceChangeResponse{NewState: state, Diagnostics: DiagnosticsFromTFProto(resp.Diagnostics)}, diagnosticsError(resp.Diagnostics)
}

func (a *V6Adapter) CreateResource(ctx context.Context, req *runtime.CreateResourceRequest) (*runtime.ResourceResult, error) {
	plan, err := a.PlanResourceChange(ctx, &runtime.PlanResourceChangeRequest{
		TypeName:      req.TypeName,
		ProposedState: req.Config,
		Config:        req.Config,
	})
	if err != nil {
		return nil, &runtime.ResourceOperationError{Phase: "plan", Err: err}
	}
	apply, err := a.ApplyResourceChange(ctx, &runtime.ApplyResourceChangeRequest{
		TypeName:     req.TypeName,
		PlannedState: plan.PlannedState,
		Config:       req.Config,
	})
	if err != nil {
		return nil, &runtime.ResourceOperationError{Phase: "apply", RemotePossiblyChanged: true, Err: err}
	}
	return &runtime.ResourceResult{
		NewState:    apply.NewState,
		Diagnostics: append(plan.Diagnostics, apply.Diagnostics...),
		SchemaHash:  req.SchemaHash,
	}, nil
}

func (a *V6Adapter) ReadResourceState(ctx context.Context, req *runtime.ReadResourceStateRequest) (*runtime.ResourceResult, error) {
	read, err := a.ReadResource(ctx, &runtime.ReadResourceRequest{TypeName: req.TypeName, CurrentState: req.PriorState})
	if err != nil {
		return nil, &runtime.ResourceOperationError{Phase: "read", Err: err}
	}
	return &runtime.ResourceResult{
		NewState:    read.NewState,
		Diagnostics: read.Diagnostics,
		SchemaHash:  req.SchemaHash,
		NotFound:    len(read.NewState.Envelope) == 0,
	}, nil
}

func (a *V6Adapter) UpdateResource(ctx context.Context, req *runtime.UpdateResourceRequest) (*runtime.ResourceResult, error) {
	plan, err := a.PlanResourceChange(ctx, &runtime.PlanResourceChangeRequest{
		TypeName:      req.TypeName,
		PriorState:    req.PriorState,
		ProposedState: req.ProposedConfig,
		Config:        req.Config,
	})
	if err != nil {
		return nil, &runtime.ResourceOperationError{Phase: "plan", Err: err}
	}
	apply, err := a.ApplyResourceChange(ctx, &runtime.ApplyResourceChangeRequest{
		TypeName:     req.TypeName,
		PriorState:   req.PriorState,
		PlannedState: plan.PlannedState,
		Config:       req.Config,
	})
	if err != nil {
		return nil, &runtime.ResourceOperationError{Phase: "apply", RemotePossiblyChanged: true, Err: err}
	}
	return &runtime.ResourceResult{
		NewState:    apply.NewState,
		Diagnostics: append(plan.Diagnostics, apply.Diagnostics...),
		ReplacementInfo: runtime.ReplacementInfo{
			Required:  len(plan.ReplacementMetadata.RequiresReplace) > 0,
			Performed: len(plan.ReplacementMetadata.RequiresReplace) > 0,
		},
		SchemaHash: req.SchemaHash,
	}, nil
}

func (a *V6Adapter) DeleteResource(ctx context.Context, req *runtime.DeleteResourceRequest) (*runtime.ResourceResult, error) {
	nullValue := runtime.DynamicValue{JSON: []byte("null")}
	plan, err := a.PlanResourceChange(ctx, &runtime.PlanResourceChangeRequest{
		TypeName:      req.TypeName,
		PriorState:    req.PriorState,
		ProposedState: nullValue,
		Config:        nullValue,
	})
	if err != nil {
		return nil, &runtime.ResourceOperationError{Phase: "plan", Err: err}
	}
	apply, err := a.ApplyResourceChange(ctx, &runtime.ApplyResourceChangeRequest{
		TypeName:     req.TypeName,
		PriorState:   req.PriorState,
		PlannedState: plan.PlannedState,
		Config:       nullValue,
	})
	if err != nil {
		return nil, &runtime.ResourceOperationError{Phase: "apply", RemotePossiblyChanged: true, Err: err}
	}
	return &runtime.ResourceResult{
		NewState:    apply.NewState,
		Diagnostics: append(plan.Diagnostics, apply.Diagnostics...),
		SchemaHash:  req.SchemaHash,
		Deleted:     len(apply.NewState.Envelope) == 0,
	}, nil
}

func (a *V6Adapter) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	resp, err := a.client.StopProvider(ctx, &tfprotov6.StopProviderRequest{})
	if err != nil {
		return err
	}
	if resp != nil && resp.Error != "" {
		return fmt.Errorf("stop provider: %s", resp.Error)
	}
	return nil
}

func (a *V6Adapter) ensureSchema(ctx context.Context) error {
	if a.schema != nil {
		return nil
	}
	_, err := a.Schema(ctx)
	return err
}

func (a *V6Adapter) dynamicFromJSON(typ tftypes.Type, value runtime.DynamicValue) (*tfprotov6.DynamicValue, error) {
	if len(value.JSON) == 0 {
		value.JSON = []byte("null")
	}
	return JSONToDynamicValue(typ, value.JSON)
}

func (a *V6Adapter) dynamicToJSON(typ tftypes.Type, value *tfprotov6.DynamicValue) (runtime.DynamicValue, error) {
	if value == nil {
		return runtime.DynamicValue{JSON: []byte("null")}, nil
	}
	raw, err := DynamicValueToJSON(typ, value)
	if err != nil {
		return runtime.DynamicValue{}, err
	}
	return runtime.DynamicValue{JSON: raw}, nil
}

func (a *V6Adapter) stateFromDynamic(typeName string, value *tfprotov6.DynamicValue, private []byte, identity *tfprotov6.ResourceIdentityData) (runtime.ProviderState, error) {
	dynamic, err := a.dynamicToJSON(a.resourceType(typeName), value)
	if err != nil {
		return runtime.ProviderState{}, err
	}
	identityJSON, err := a.identityToJSON(typeName, identity)
	if err != nil {
		return runtime.ProviderState{}, err
	}

	envelope := runtime.StateEnvelope{
		FormatVersion: 1,
		Protocol:      protocolName,
		TypeName:      typeName,
		SchemaVersion: a.resourceVersion(typeName),
		Value:         dynamic,
		Private:       private,
		Identity:      identityJSON,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return runtime.ProviderState{}, err
	}
	return runtime.ProviderState{Envelope: raw}, nil
}

func providerStateParts(state runtime.ProviderState) ([]byte, []byte, []byte, int64, error) {
	if len(state.Envelope) == 0 {
		return []byte("null"), nil, nil, 0, nil
	}
	var envelope runtime.StateEnvelope
	if err := json.Unmarshal(state.Envelope, &envelope); err != nil {
		return nil, nil, nil, 0, fmt.Errorf("decode provider state envelope: %w", err)
	}
	if envelope.Protocol != "" && envelope.Protocol != "tfprotov6" {
		return nil, nil, nil, 0, fmt.Errorf("state envelope protocol %q incompatible with v6 adapter", envelope.Protocol)
	}
	if len(envelope.Value.JSON) == 0 {
		envelope.Value.JSON = []byte("null")
	}
	return envelope.Value.JSON, envelope.Private, envelope.Identity, envelope.SchemaVersion, nil
}

func privateOrFallback(envelopePrivate []byte, requestPrivate []byte) []byte {
	if len(envelopePrivate) == 0 {
		return requestPrivate
	}
	return envelopePrivate
}

func (a *V6Adapter) identityFromJSON(typeName string, raw []byte) (*tfprotov6.ResourceIdentityData, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	value, err := a.dynamicFromJSON(a.identityType(typeName), runtime.DynamicValue{JSON: raw})
	if err != nil {
		return nil, err
	}
	return &tfprotov6.ResourceIdentityData{IdentityData: value}, nil
}

func (a *V6Adapter) identityToJSON(typeName string, identity *tfprotov6.ResourceIdentityData) ([]byte, error) {
	if identity == nil || identity.IdentityData == nil {
		return nil, nil
	}
	value, err := a.dynamicToJSON(a.identityType(typeName), identity.IdentityData)
	if err != nil {
		return nil, err
	}
	return value.JSON, nil
}

func diagnosticsError(diags []*tfprotov6.Diagnostic) error {
	translated := DiagnosticsFromTFProto(diags)
	for _, diag := range translated {
		if diag.Severity == runtime.DiagError {
			return &runtime.ProviderDiagnosticError{Diagnostics: translated}
		}
	}
	return nil
}

func toTFDiagnostics(diags []runtime.ProviderDiagnostic) []*tfprotov6.Diagnostic {
	out := make([]*tfprotov6.Diagnostic, 0, len(diags))
	for _, diag := range diags {
		severity := tfprotov6.DiagnosticSeverityError
		if diag.Severity == runtime.DiagWarning {
			severity = tfprotov6.DiagnosticSeverityWarning
		}
		out = append(out, &tfprotov6.Diagnostic{Severity: severity, Summary: diag.Summary, Detail: diag.Detail})
	}
	return out
}

func formatAttributePaths(paths []*tftypes.AttributePath) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, formatAttributePath(path))
	}
	return out
}

func marshalSchemaBlock(schema *tfprotov6.Schema) []byte {
	if schema == nil || schema.Block == nil {
		return nil
	}
	block, err := mapSchemaBlock(schema.Block)
	if err != nil {
		return nil
	}
	raw, _ := json.Marshal(block)
	return raw
}

type mappedBlock struct {
	Description string                     `json:"description,omitempty"`
	Attributes  map[string]mappedAttribute `json:"attributes,omitempty"`
	BlockTypes  map[string]mappedNested    `json:"block_types,omitempty"`
}

type mappedAttribute struct {
	Type        json.RawMessage `json:"type,omitempty"`
	NestedType  *mappedObject   `json:"nested_type,omitempty"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Optional    bool            `json:"optional,omitempty"`
	Computed    bool            `json:"computed,omitempty"`
	Sensitive   bool            `json:"sensitive,omitempty"`
	WriteOnly   bool            `json:"write_only,omitempty"`
}

type mappedObject struct {
	NestingMode string                     `json:"nesting_mode"`
	Attributes  map[string]mappedAttribute `json:"attributes"`
}

type mappedNested struct {
	NestingMode string      `json:"nesting_mode"`
	Block       mappedBlock `json:"block"`
	MinItems    int         `json:"min_items,omitempty"`
	MaxItems    int         `json:"max_items,omitempty"`
}

func mapSchemaBlock(block *tfprotov6.SchemaBlock) (mappedBlock, error) {
	if block == nil {
		return mappedBlock{}, nil
	}
	out := mappedBlock{Description: block.Description}
	if len(block.Attributes) > 0 {
		out.Attributes = make(map[string]mappedAttribute, len(block.Attributes))
	}
	for _, attr := range block.Attributes {
		if attr == nil {
			continue
		}
		mapped, err := mapSchemaAttribute(attr)
		if err != nil {
			return mappedBlock{}, err
		}
		out.Attributes[attr.Name] = mapped
	}
	if len(block.BlockTypes) > 0 {
		out.BlockTypes = make(map[string]mappedNested, len(block.BlockTypes))
	}
	for _, nested := range block.BlockTypes {
		if nested == nil {
			continue
		}
		mapped, err := mapSchemaBlock(nested.Block)
		if err != nil {
			return mappedBlock{}, err
		}
		out.BlockTypes[nested.TypeName] = mappedNested{
			NestingMode: schemaBlockMode(nested.Nesting.String()),
			Block:       mapped,
			MinItems:    int(nested.MinItems),
			MaxItems:    int(nested.MaxItems),
		}
	}
	return out, nil
}

func mapSchemaAttribute(attr *tfprotov6.SchemaAttribute) (mappedAttribute, error) {
	out := mappedAttribute{
		Description: attr.Description,
		Required:    attr.Required,
		Optional:    attr.Optional,
		Computed:    attr.Computed,
		Sensitive:   attr.Sensitive,
		WriteOnly:   attr.WriteOnly,
	}
	if attr.NestedType != nil {
		nested := &mappedObject{
			NestingMode: schemaObjectMode(attr.NestedType.Nesting.String()),
			Attributes:  make(map[string]mappedAttribute, len(attr.NestedType.Attributes)),
		}
		for _, child := range attr.NestedType.Attributes {
			if child == nil {
				continue
			}
			mapped, err := mapSchemaAttribute(child)
			if err != nil {
				return mappedAttribute{}, err
			}
			nested.Attributes[child.Name] = mapped
		}
		out.NestedType = nested
		return out, nil
	}
	typeJSON, err := attr.Type.MarshalJSON() //nolint:staticcheck
	if err != nil {
		return mappedAttribute{}, err
	}
	out.Type = typeJSON
	return out, nil
}

func schemaBlockMode(mode string) string {
	switch mode {
	case "SINGLE", "GROUP":
		return "single"
	case "LIST":
		return "list"
	case "SET":
		return "set"
	case "MAP":
		return "map"
	default:
		return "invalid"
	}
}

func schemaObjectMode(mode string) string {
	switch mode {
	case "SINGLE":
		return "single"
	case "LIST":
		return "list"
	case "SET":
		return "set"
	case "MAP":
		return "map"
	default:
		return "invalid"
	}
}

func schemaVersion(schema *tfprotov6.Schema) int64 {
	if schema == nil {
		return 0
	}
	return schema.Version
}

type schemaCache struct {
	provider    *tfprotov6.Schema
	resources   map[string]*tfprotov6.Schema
	dataSources map[string]*tfprotov6.Schema
	actions     map[string]*tfprotov6.Schema
	identities  map[string]*tfprotov6.ResourceIdentitySchema
}

func newSchemaCache(resp *tfprotov6.GetProviderSchemaResponse, identityResp *tfprotov6.GetResourceIdentitySchemasResponse) *schemaCache {
	cache := &schemaCache{
		resources:   resp.ResourceSchemas,
		dataSources: resp.DataSourceSchemas,
		actions:     make(map[string]*tfprotov6.Schema, len(resp.ActionSchemas)),
		provider:    resp.Provider,
	}
	if identityResp != nil {
		cache.identities = identityResp.IdentitySchemas
	}
	for name, schema := range resp.ActionSchemas {
		cache.actions[name] = schema.Schema
	}
	return cache
}

func (a *V6Adapter) providerType() tftypes.Type {
	if a.schema == nil {
		return tftypes.DynamicPseudoType
	}
	return a.schema.provider.ValueType()
}

func (a *V6Adapter) resourceType(name string) tftypes.Type {
	if a.schema == nil || a.schema.resources[name] == nil {
		return tftypes.DynamicPseudoType
	}
	return a.schema.resources[name].ValueType()
}

func (a *V6Adapter) resourceVersion(name string) int64 {
	if a.schema == nil {
		return 0
	}
	return schemaVersion(a.schema.resources[name])
}

func (a *V6Adapter) dataSourceType(name string) tftypes.Type {
	if a.schema == nil || a.schema.dataSources[name] == nil {
		return tftypes.DynamicPseudoType
	}
	return a.schema.dataSources[name].ValueType()
}

func (a *V6Adapter) actionType(name string) tftypes.Type {
	if a.schema == nil || a.schema.actions[name] == nil {
		return tftypes.DynamicPseudoType
	}
	return a.schema.actions[name].ValueType()
}

func (a *V6Adapter) identityType(name string) tftypes.Type {
	if a.schema == nil || a.schema.identities[name] == nil {
		return tftypes.DynamicPseudoType
	}
	return a.schema.identities[name].ValueType()
}
