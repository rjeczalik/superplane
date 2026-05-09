package runtime

import (
	"context"
	"fmt"
)

type ConfigureRequest struct {
	Config DynamicValue
}

type PrepareProviderConfigRequest struct {
	Config DynamicValue
}

type PrepareProviderConfigResponse struct {
	PreparedConfig DynamicValue
	Diagnostics    []ProviderDiagnostic
}

type ValidateProviderConfigRequest struct {
	Config DynamicValue
}

type ValidateDataSourceConfigRequest struct {
	TypeName string
	Config   DynamicValue
}

type ReadDataSourceRequest struct {
	TypeName string
	Config   DynamicValue
}

type ReadDataSourceResponse struct {
	State       DynamicValue
	Diagnostics []ProviderDiagnostic
}

type ValidateActionRequest struct {
	TypeName string
	Config   DynamicValue
}

type PlanActionRequest struct {
	TypeName string
	Config   DynamicValue
}

type PlanActionResponse struct {
	Diagnostics []ProviderDiagnostic
	Deferred    bool
}

type InvokeActionRequest struct {
	TypeName string
	Config   DynamicValue
}

type InvokeActionResponse struct {
	Events      []ActionEvent
	Diagnostics []ProviderDiagnostic
}

type ValidateResourceConfigRequest struct {
	TypeName string
	Config   DynamicValue
}

type ReadResourceRequest struct {
	TypeName     string
	CurrentState ProviderState
}

type ReadResourceResponse struct {
	NewState    ProviderState
	Diagnostics []ProviderDiagnostic
}

type PlanResourceChangeRequest struct {
	TypeName      string
	PriorState    ProviderState
	ProposedState DynamicValue
	Config        DynamicValue
	PriorPrivate  []byte
}

type PlanResourceChangeResponse struct {
	PlannedState        ProviderState
	ReplacementMetadata ReplacementMetadata
	Diagnostics         []ProviderDiagnostic
}

type ApplyResourceChangeRequest struct {
	TypeName       string
	PriorState     ProviderState
	PlannedState   ProviderState
	Config         DynamicValue
	PlannedPrivate []byte
}

type ApplyResourceChangeResponse struct {
	NewState    ProviderState
	Diagnostics []ProviderDiagnostic
}

type CreateResourceRequest struct {
	TypeName   string
	Config     DynamicValue
	SchemaHash string
}

type ReadResourceStateRequest struct {
	TypeName   string
	PriorState ProviderState
	SchemaHash string
}

type UpdateResourceRequest struct {
	TypeName       string
	PriorState     ProviderState
	Config         DynamicValue
	ProposedConfig DynamicValue
	SchemaHash     string
}

type DeleteResourceRequest struct {
	TypeName   string
	PriorState ProviderState
	SchemaHash string
}

type ResourceResult struct {
	NewState        ProviderState
	Diagnostics     []ProviderDiagnostic
	ReplacementInfo ReplacementInfo
	SchemaHash      string
	Deleted         bool
	NotFound        bool
}

type ReplacementInfo struct {
	Required         bool
	Performed        bool
	PreviousRemoteID string
	NewRemoteID      string
}

type ResourceOperationError struct {
	Phase                 string
	RemotePossiblyChanged bool
	Diagnostics           []ProviderDiagnostic
	Err                   error
}

func (e *ResourceOperationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("terraform resource operation failed during %s: %v", e.Phase, e.Err)
	}
	return fmt.Sprintf("terraform resource operation failed during %s", e.Phase)
}

func (e *ResourceOperationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type SchemaResponse struct {
	Provider    ProviderSchemaBlock
	Resources   map[string]SchemaBlock
	DataSources map[string]SchemaBlock
	Actions     map[string]SchemaBlock
}

type ProviderSchemaBlock struct {
	Block []byte
}

type SchemaBlock struct {
	Version       int64
	Block         []byte
	HasPlanStep   bool
	StreamsEvents bool
}

// ProviderRuntime.Schema returns SchemaResponse populated by protocol adapters
// from tfplugin5/tfplugin6 RPCs. SchemaMapper converts SchemaResponse into
// CapabilitySchema and existing terraform.Block-shaped schema bytes before
// CapabilityMapper builds generated actions/capabilities.
type ProviderRuntime interface {
	Schema(ctx context.Context) (*SchemaResponse, error)
	PrepareProviderConfig(ctx context.Context, req *PrepareProviderConfigRequest) (*PrepareProviderConfigResponse, error)
	ValidateProviderConfig(ctx context.Context, req *ValidateProviderConfigRequest) error
	Configure(ctx context.Context, req *ConfigureRequest) error
	ValidateDataSourceConfig(ctx context.Context, req *ValidateDataSourceConfigRequest) error
	ReadDataSource(ctx context.Context, req *ReadDataSourceRequest) (*ReadDataSourceResponse, error)
	ValidateAction(ctx context.Context, req *ValidateActionRequest) error
	PlanAction(ctx context.Context, req *PlanActionRequest) (*PlanActionResponse, error)
	InvokeAction(ctx context.Context, req *InvokeActionRequest) (*InvokeActionResponse, error)
	ValidateResourceConfig(ctx context.Context, req *ValidateResourceConfigRequest) error
	ReadResource(ctx context.Context, req *ReadResourceRequest) (*ReadResourceResponse, error)
	PlanResourceChange(ctx context.Context, req *PlanResourceChangeRequest) (*PlanResourceChangeResponse, error)
	ApplyResourceChange(ctx context.Context, req *ApplyResourceChangeRequest) (*ApplyResourceChangeResponse, error)
	CreateResource(ctx context.Context, req *CreateResourceRequest) (*ResourceResult, error)
	ReadResourceState(ctx context.Context, req *ReadResourceStateRequest) (*ResourceResult, error)
	UpdateResource(ctx context.Context, req *UpdateResourceRequest) (*ResourceResult, error)
	DeleteResource(ctx context.Context, req *DeleteResourceRequest) (*ResourceResult, error)
	Close() error
}

type SchemaReader interface {
	Schema(ctx context.Context) (*CapabilitySchema, error)
}

type DataSourceReader interface {
	Configure(ctx context.Context, req *ConfigureRequest) error
	ValidateDataSourceConfig(ctx context.Context, req *ValidateDataSourceConfigRequest) error
	ReadDataSource(ctx context.Context, req *ReadDataSourceRequest) (*ReadDataSourceResponse, error)
}

type ResourceLifecycle interface {
	Configure(ctx context.Context, req *ConfigureRequest) error
	ValidateResourceConfig(ctx context.Context, req *ValidateResourceConfigRequest) error
	ReadResource(ctx context.Context, req *ReadResourceRequest) (*ReadResourceResponse, error)
	PlanResourceChange(ctx context.Context, req *PlanResourceChangeRequest) (*PlanResourceChangeResponse, error)
	ApplyResourceChange(ctx context.Context, req *ApplyResourceChangeRequest) (*ApplyResourceChangeResponse, error)
}
