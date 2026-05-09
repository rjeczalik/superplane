package workers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/config"
	terraformintegration "github.com/superplanehq/superplane/pkg/integrations/terraform"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/datatypes"
)

func TestTerraformPollingWorkerRefreshesResource(t *testing.T) {
	db := terraformEventDispatcherTestDB(t)
	fixture := terraformEventDispatcherFixture(t, db)
	envelope := pollingStateEnvelope(t, map[string]any{"id": "remote-1"})
	store := &pollingStore{
		operationID: uuid.New(),
		loaded: &terraformintegration.LoadedManagedResource{
			Resource:     fixture.resource,
			StatePayload: envelope,
			State:        models.TerraformManagedResourceState{LockVersion: 2, SchemaHash: "hash", StateFormat: terraformintegration.TerraformStateFormatRuntime},
		},
	}
	rt := &pollingRuntime{
		readResourceState: &runtime.ResourceResult{NewState: runtime.ProviderState{Envelope: pollingStateEnvelope(t, map[string]any{"id": "remote-2"})}},
	}
	worker := NewTerraformPollingWorker(store, &pollingRuntimeFactory{runtime: rt}, pollingProviderConfigLoader{})

	require.NoError(t, worker.RefreshResource(context.Background(), fixture.resource.ManagedResourceID))
	assert.True(t, store.saveRefreshedCalled)
	assert.Equal(t, []string{"Configure", "ReadResourceState", "Close"}, rt.calls)

	events, err := models.ListPendingManagedResourceEvents(10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, models.ManagedResourceEventUpdated, events[0].EventType)
	assert.Equal(t, "remote-2", events[0].Outputs.Data()["id"])
}

func pollingStateEnvelope(t *testing.T, payload map[string]any) []byte {
	t.Helper()
	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)
	raw, err := json.Marshal(runtime.StateEnvelope{Value: runtime.DynamicValue{JSON: rawPayload}})
	require.NoError(t, err)
	return raw
}

type pollingProviderConfigLoader struct{}

func (pollingProviderConfigLoader) LoadProviderConfig(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}

type pollingRuntimeFactory struct {
	runtime runtime.ProviderRuntime
}

func (f *pollingRuntimeFactory) RuntimeForProvider(context.Context, config.TerraformProviderIntegration) (runtime.ProviderRuntime, error) {
	return f.runtime, nil
}

type pollingStore struct {
	operationID         uuid.UUID
	loaded              *terraformintegration.LoadedManagedResource
	saveRefreshedCalled bool
}

func (s *pollingStore) BeginCreate(context.Context, terraformintegration.ManagedResourceAuthContext, terraformintegration.BeginManagedResourceCreateInput) (*models.TerraformManagedResource, error) {
	return nil, nil
}
func (s *pollingStore) CompleteCreate(context.Context, terraformintegration.ManagedResourceAuthContext, terraformintegration.CompleteManagedResourceCreateInput) error {
	return nil
}
func (s *pollingStore) MarkCreateProviderFailed(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (s *pollingStore) MarkCreateOrphanRisk(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, uuid.UUID, string, map[string]any) error {
	return nil
}
func (s *pollingStore) FindExistingForIdempotency(context.Context, terraformintegration.ManagedResourceAuthContext, string, string) (*models.TerraformManagedResource, error) {
	return nil, nil
}
func (s *pollingStore) ClaimOperation(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, string, time.Time, []string) (uuid.UUID, error) {
	return s.operationID, nil
}
func (s *pollingStore) RefreshOperationLease(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}
func (s *pollingStore) MarkOperationFailed(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, uuid.UUID, string, string) error {
	return nil
}
func (s *pollingStore) Load(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID) (*terraformintegration.LoadedManagedResource, error) {
	return s.loaded, nil
}
func (s *pollingStore) LoadForOperation(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, uuid.UUID) (*terraformintegration.LoadedManagedResource, error) {
	return s.loaded, nil
}
func (s *pollingStore) SaveState(context.Context, terraformintegration.ManagedResourceAuthContext, terraformintegration.SaveManagedResourceStateInput) error {
	return nil
}
func (s *pollingStore) SaveRefreshedState(context.Context, terraformintegration.ManagedResourceAuthContext, terraformintegration.SaveManagedResourceStateInput) error {
	s.saveRefreshedCalled = true
	return nil
}
func (s *pollingStore) RecordMissing(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, uuid.UUID, int, map[string]any) (string, error) {
	return models.ManagedResourceEventMissing, nil
}
func (s *pollingStore) Delete(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *pollingStore) ForceForget(context.Context, terraformintegration.ManagedResourceAuthContext, uuid.UUID, uuid.UUID) error {
	return nil
}

type pollingRuntime struct {
	calls             []string
	readResourceState *runtime.ResourceResult
}

func (r *pollingRuntime) record(call string) { r.calls = append(r.calls, call) }
func (r *pollingRuntime) Schema(context.Context) (*runtime.SchemaResponse, error) {
	return nil, nil
}
func (r *pollingRuntime) PrepareProviderConfig(context.Context, *runtime.PrepareProviderConfigRequest) (*runtime.PrepareProviderConfigResponse, error) {
	return nil, nil
}
func (r *pollingRuntime) ValidateProviderConfig(context.Context, *runtime.ValidateProviderConfigRequest) error {
	return nil
}
func (r *pollingRuntime) Configure(context.Context, *runtime.ConfigureRequest) error {
	r.record("Configure")
	return nil
}
func (r *pollingRuntime) ValidateDataSourceConfig(context.Context, *runtime.ValidateDataSourceConfigRequest) error {
	return nil
}
func (r *pollingRuntime) ReadDataSource(context.Context, *runtime.ReadDataSourceRequest) (*runtime.ReadDataSourceResponse, error) {
	return nil, nil
}
func (r *pollingRuntime) ValidateAction(context.Context, *runtime.ValidateActionRequest) error {
	return nil
}
func (r *pollingRuntime) PlanAction(context.Context, *runtime.PlanActionRequest) (*runtime.PlanActionResponse, error) {
	return nil, nil
}
func (r *pollingRuntime) InvokeAction(context.Context, *runtime.InvokeActionRequest) (*runtime.InvokeActionResponse, error) {
	return nil, nil
}
func (r *pollingRuntime) ValidateResourceConfig(context.Context, *runtime.ValidateResourceConfigRequest) error {
	return nil
}
func (r *pollingRuntime) ReadResource(context.Context, *runtime.ReadResourceRequest) (*runtime.ReadResourceResponse, error) {
	return nil, nil
}
func (r *pollingRuntime) PlanResourceChange(context.Context, *runtime.PlanResourceChangeRequest) (*runtime.PlanResourceChangeResponse, error) {
	return nil, nil
}
func (r *pollingRuntime) ApplyResourceChange(context.Context, *runtime.ApplyResourceChangeRequest) (*runtime.ApplyResourceChangeResponse, error) {
	return nil, nil
}
func (r *pollingRuntime) CreateResource(context.Context, *runtime.CreateResourceRequest) (*runtime.ResourceResult, error) {
	return nil, nil
}
func (r *pollingRuntime) ReadResourceState(context.Context, *runtime.ReadResourceStateRequest) (*runtime.ResourceResult, error) {
	r.record("ReadResourceState")
	return r.readResourceState, nil
}
func (r *pollingRuntime) UpdateResource(context.Context, *runtime.UpdateResourceRequest) (*runtime.ResourceResult, error) {
	return nil, nil
}
func (r *pollingRuntime) DeleteResource(context.Context, *runtime.DeleteResourceRequest) (*runtime.ResourceResult, error) {
	return nil, nil
}
func (r *pollingRuntime) Close() error {
	r.record("Close")
	return nil
}

var _ = datatypes.JSONType[map[string]any]{}
