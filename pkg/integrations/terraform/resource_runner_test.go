package terraform

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/gorm"
)

func TestResourceRunnerCreate(t *testing.T) {
	rt := &fakeDirectRuntime{
		createResource: &runtime.ResourceResult{
			NewState: runtime.ProviderState{Envelope: []byte(`{"state":1}`)},
		},
	}
	store := &memoryManagedResourceStore{}
	runner := NewResourceRunner(store, &singleRuntimeFactory{runtime: rt}, time.Minute)
	action := runnerTestAction()
	action.op = "create"

	execState := &recordingExecutionState{}
	err := runner.Create(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"machine_type": "init"},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	}, action)
	require.NoError(t, err)
	require.True(t, store.beginCalled)
	require.True(t, store.completeCalled)
	require.Nil(t, store.beginInput.IdempotencyKey)
	require.Equal(t, []string{"Configure", "ValidateResourceConfig", "CreateResource", "Close"}, rt.calls)
	require.Len(t, execState.payloads, 1)
}

func TestResourceRunnerCreateUsesIdempotencyControls(t *testing.T) {
	rt := &fakeDirectRuntime{
		createResource: &runtime.ResourceResult{
			NewState: runtime.ProviderState{Envelope: marshalStateEnvelope(t, map[string]any{"id": "remote-1"})},
		},
	}
	store := &memoryManagedResourceStore{}
	runner := NewResourceRunner(store, &singleRuntimeFactory{runtime: rt}, time.Minute)
	action := runnerTestAction()
	action.op = "create"

	err := runner.Create(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"machine_type": "init", "idempotency_key": "cluster-a", "on_existing": "fail"},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: &recordingExecutionState{},
	}, action)
	require.NoError(t, err)
	require.NotNil(t, store.beginInput.IdempotencyKey)
	require.Equal(t, "cluster-a", *store.beginInput.IdempotencyKey)
	require.JSONEq(t, `{"machine_type":"init"}`, string(store.completeInput.ConfigPayload))
}

func TestResourceRunnerCreateMarksProviderFailureWhenRuntimeCannotStart(t *testing.T) {
	store := &memoryManagedResourceStore{}
	runner := NewResourceRunner(store, &singleRuntimeFactory{err: errors.New("runtime unavailable")}, time.Minute)
	action := runnerTestAction()
	action.op = "create"

	err := runner.Create(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"machine_type": "init"},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: &recordingExecutionState{},
	}, action)

	require.Error(t, err)
	require.True(t, store.beginCalled)
	require.True(t, store.markCreateProviderFailedCalled)
	require.False(t, store.markCreateOrphanRiskCalled)
}

func TestResourceRunnerRead(t *testing.T) {
	resourceID := uuid.New()
	operationID := uuid.New()
	stateEnvelope := marshalStateEnvelope(t, map[string]any{"id": "remote-1"})
	rt := &fakeDirectRuntime{
		readResourceState: &runtime.ResourceResult{
			NewState: runtime.ProviderState{Envelope: stateEnvelope},
		},
	}
	store := &memoryManagedResourceStore{
		operationID: operationID,
		loaded: &LoadedManagedResource{
			Resource: models.TerraformManagedResource{
				ManagedResourceID: resourceID,
				ResourceType:      "talos_machine_secrets",
				ProviderName:      "talos",
				ProviderSource:    "registry.terraform.io/siderolabs/talos",
				ProviderVersion:   "0.11.0",
			},
			StatePayload: stateEnvelope,
			State:        models.TerraformManagedResourceState{LockVersion: 0},
		},
	}
	runner := NewResourceRunner(store, &singleRuntimeFactory{runtime: rt}, time.Minute)
	action := runnerTestAction()
	action.op = "read"

	execState := &recordingExecutionState{}
	err := runner.Read(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"managed_resource_id": resourceID.String()},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	}, action)
	require.NoError(t, err)
	require.True(t, store.saveRefreshedCalled)
	require.Equal(t, []string{"Configure", "ReadResourceState", "Close"}, rt.calls)
	require.Len(t, execState.payloads, 1)
}

func TestResourceRunnerUpdateClearsOperationWhenRefreshFails(t *testing.T) {
	resourceID := uuid.New()
	operationID := uuid.New()
	stateEnvelope := marshalStateEnvelope(t, map[string]any{"id": "remote-1"})
	rt := &fakeDirectRuntime{readResourceStateErr: errors.New("provider read failed")}
	store := &memoryManagedResourceStore{
		operationID: operationID,
		loaded: &LoadedManagedResource{
			Resource: models.TerraformManagedResource{
				ManagedResourceID: resourceID,
				ResourceType:      "talos_machine_secrets",
				ProviderName:      "talos",
				ProviderSource:    "registry.terraform.io/siderolabs/talos",
				ProviderVersion:   "0.11.0",
				Status:            models.ManagedResourceStatusReady,
			},
			StatePayload:  stateEnvelope,
			ConfigPayload: []byte(`{"machine_type":"controlplane"}`),
			State:         models.TerraformManagedResourceState{LockVersion: 1},
		},
	}
	runner := NewResourceRunner(store, &singleRuntimeFactory{runtime: rt}, time.Minute)
	action := runnerTestAction()
	action.op = "update"

	err := runner.Update(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"managed_resource_id": resourceID.String(), "machine_type": "worker"},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: &recordingExecutionState{},
	}, action)

	require.Error(t, err)
	require.True(t, store.markOperationFailedCalled)
	require.Equal(t, models.ManagedResourceStatusReady, store.markOperationFailedStatus)
}

func TestResourceRunnerUpdateRecordsStateAndEventPayload(t *testing.T) {
	resourceID := uuid.New()
	operationID := uuid.New()
	stateEnvelope := marshalStateEnvelope(t, map[string]any{"id": "remote-1", "version": "new"})
	rt := &fakeDirectRuntime{
		planResource:  &runtime.PlanResourceChangeResponse{PlannedState: runtime.ProviderState{Envelope: stateEnvelope}},
		applyResource: &runtime.ApplyResourceChangeResponse{NewState: runtime.ProviderState{Envelope: stateEnvelope}},
	}
	store := &memoryManagedResourceStore{
		operationID: operationID,
		loaded: &LoadedManagedResource{
			Resource: models.TerraformManagedResource{
				ManagedResourceID: resourceID,
				ResourceType:      "talos_machine_secrets",
				ProviderName:      "talos",
				ProviderSource:    "registry.terraform.io/siderolabs/talos",
				ProviderVersion:   "0.11.0",
			},
			StatePayload:  stateEnvelope,
			ConfigPayload: []byte(`{"machine_type":"controlplane","version":"old","nested":{"keep":true,"change":"old"}}`),
			State:         models.TerraformManagedResourceState{LockVersion: 1},
		},
	}
	runner := NewResourceRunner(store, &singleRuntimeFactory{runtime: rt}, time.Minute)
	action := runnerTestAction()
	action.op = "update"

	execState := &recordingExecutionState{}
	err := runner.Update(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"managed_resource_id": resourceID.String(), "version": "new", "nested": map[string]any{"change": "new"}},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	}, action)
	require.NoError(t, err)
	require.True(t, store.saveCalled)
	require.True(t, store.saveRefreshedCalled)
	require.Equal(t, []string{"Configure", "ReadResourceState", "PlanResourceChange", "ApplyResourceChange", "Close"}, rt.calls)
	require.JSONEq(t, `{"machine_type":"controlplane","version":"new","nested":{"keep":true,"change":"new"}}`, string(store.savedInput.ConfigPayload))
	require.Len(t, execState.payloads, 1)
}

func TestResourceRunnerDeleteRequiresConfirmationAndDeletesProviderResource(t *testing.T) {
	resourceID := uuid.New()
	operationID := uuid.New()
	stateEnvelope := marshalStateEnvelope(t, map[string]any{"id": "remote-1"})
	rt := &fakeDirectRuntime{
		deleteResource: &runtime.ResourceResult{
			NewState: runtime.ProviderState{Envelope: stateEnvelope},
		},
	}
	store := &memoryManagedResourceStore{
		operationID: operationID,
		loaded: &LoadedManagedResource{
			Resource: models.TerraformManagedResource{
				ManagedResourceID: resourceID,
				ResourceType:      "talos_machine_secrets",
				ProviderName:      "talos",
				ProviderSource:    "registry.terraform.io/siderolabs/talos",
				ProviderVersion:   "0.11.0",
			},
			StatePayload: stateEnvelope,
			State:        models.TerraformManagedResourceState{LockVersion: 1},
		},
	}
	runner := NewResourceRunner(store, &singleRuntimeFactory{runtime: rt}, time.Minute)
	action := runnerTestAction()
	action.op = "delete"

	execState := &recordingExecutionState{}
	err := runner.Delete(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"managed_resource_id": resourceID.String(), "confirm_delete": true},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	}, action)
	require.NoError(t, err)
	require.True(t, store.deleteCalled)
	require.Equal(t, []string{"Configure", "DeleteResource", "Close"}, rt.calls)
	require.Len(t, execState.payloads, 1)
}

func TestResourceRunnerUpdatePreviewOnlyDoesNotApplyOrSaveConfig(t *testing.T) {
	resourceID := uuid.New()
	operationID := uuid.New()
	stateEnvelope := marshalStateEnvelope(t, map[string]any{"id": "remote-1", "version": "old"})
	rt := &fakeDirectRuntime{
		planResource: &runtime.PlanResourceChangeResponse{
			PlannedState:        runtime.ProviderState{Envelope: marshalStateEnvelope(t, map[string]any{"id": "remote-1", "version": "new"})},
			ReplacementMetadata: runtime.ReplacementMetadata{RequiresReplace: []string{"version"}},
		},
	}
	store := &memoryManagedResourceStore{
		operationID: operationID,
		loaded: &LoadedManagedResource{
			Resource: models.TerraformManagedResource{
				ManagedResourceID: resourceID,
				ResourceType:      "talos_machine_secrets",
				ProviderName:      "talos",
				ProviderSource:    "registry.terraform.io/siderolabs/talos",
				ProviderVersion:   "0.11.0",
			},
			StatePayload:  stateEnvelope,
			ConfigPayload: []byte(`{"version":"old"}`),
			State:         models.TerraformManagedResourceState{LockVersion: 1},
		},
	}
	runner := NewResourceRunner(store, &singleRuntimeFactory{runtime: rt}, time.Minute)
	action := runnerTestAction()
	action.op = "update"

	execState := &recordingExecutionState{}
	err := runner.Update(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"managed_resource_id": resourceID.String(), "version": "new", "replacement_strategy": "preview_only"},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	}, action)
	require.NoError(t, err)
	require.False(t, store.saveCalled)
	require.True(t, store.saveRefreshedCalled)
	require.Equal(t, []string{"Configure", "ReadResourceState", "PlanResourceChange", "Close"}, rt.calls)
	require.Len(t, execState.payloads, 1)
	payload := execState.payloads[0].(map[string]any)
	require.Equal(t, "preview", payload["operation_performed"])
}

func TestResourceRunnerDeleteForceForgetSkipsProvider(t *testing.T) {
	resourceID := uuid.New()
	operationID := uuid.New()
	rt := &fakeDirectRuntime{}
	store := &memoryManagedResourceStore{operationID: operationID}
	runner := NewResourceRunner(store, &singleRuntimeFactory{runtime: rt}, time.Minute)
	action := runnerTestAction()
	action.op = "delete"

	execState := &recordingExecutionState{}
	err := runner.Delete(core.ExecutionContext{
		ID:             uuid.New(),
		WorkflowID:     uuid.NewString(),
		OrganizationID: uuid.NewString(),
		NodeID:         "node-1",
		Configuration:  map[string]any{"managed_resource_id": resourceID.String(), "force_forget": true, "confirm_forget": true},
		Integration: &runnerIntegrationContext{
			secrets: &mockSecretStorage{secrets: map[string]string{secretNameTerraformProviderConfig: `{}`}},
		},
		ExecutionState: execState,
	}, action)
	require.NoError(t, err)
	require.True(t, store.forceForgetCalled)
	require.Empty(t, rt.calls)
}

type singleRuntimeFactory struct {
	runtime runtime.ProviderRuntime
	err     error
}

func (f *singleRuntimeFactory) RuntimeForProvider(context.Context, config.TerraformProviderIntegration) (runtime.ProviderRuntime, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.runtime, nil
}

type memoryManagedResourceStore struct {
	beginCalled                    bool
	completeCalled                 bool
	saveCalled                     bool
	saveRefreshedCalled            bool
	deleteCalled                   bool
	forceForgetCalled              bool
	markCreateProviderFailedCalled bool
	markCreateOrphanRiskCalled     bool
	markOperationFailedCalled      bool
	markOperationFailedStatus      string
	savedInput                     SaveManagedResourceStateInput
	beginInput                     BeginManagedResourceCreateInput
	completeInput                  CompleteManagedResourceCreateInput
	existingResource               *models.TerraformManagedResource
	resource                       *models.TerraformManagedResource
	operationID                    uuid.UUID
	loaded                         *LoadedManagedResource
}

func (s *memoryManagedResourceStore) BeginCreate(ctx context.Context, auth ManagedResourceAuthContext, input BeginManagedResourceCreateInput) (*models.TerraformManagedResource, error) {
	s.beginCalled = true
	s.beginInput = input
	s.resource = &models.TerraformManagedResource{
		ManagedResourceID:  input.ManagedResourceID,
		OrganizationID:     input.OrganizationID,
		CanvasID:           input.CanvasID,
		IntegrationID:      input.IntegrationID,
		ProviderName:       input.ProviderName,
		ProviderSource:     input.ProviderSource,
		ProviderVersion:    input.ProviderVersion,
		ResourceType:       input.ResourceType,
		CurrentOperationID: &input.OperationID,
	}
	return s.resource, nil
}

func (s *memoryManagedResourceStore) CompleteCreate(ctx context.Context, auth ManagedResourceAuthContext, input CompleteManagedResourceCreateInput) error {
	s.completeCalled = true
	s.completeInput = input
	return nil
}

func (s *memoryManagedResourceStore) MarkCreateProviderFailed(context.Context, ManagedResourceAuthContext, uuid.UUID, uuid.UUID, string) error {
	s.markCreateProviderFailedCalled = true
	return nil
}
func (s *memoryManagedResourceStore) MarkCreateOrphanRisk(context.Context, ManagedResourceAuthContext, uuid.UUID, uuid.UUID, string, map[string]any) error {
	s.markCreateOrphanRiskCalled = true
	return nil
}
func (s *memoryManagedResourceStore) FindExistingForIdempotency(context.Context, ManagedResourceAuthContext, string, string) (*models.TerraformManagedResource, error) {
	if s.existingResource != nil {
		return s.existingResource, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (s *memoryManagedResourceStore) ClaimOperation(context.Context, ManagedResourceAuthContext, uuid.UUID, string, time.Time, []string) (uuid.UUID, error) {
	if s.operationID != uuid.Nil {
		return s.operationID, nil
	}
	return uuid.New(), nil
}
func (s *memoryManagedResourceStore) RefreshOperationLease(context.Context, ManagedResourceAuthContext, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}
func (s *memoryManagedResourceStore) MarkOperationFailed(_ context.Context, _ ManagedResourceAuthContext, _ uuid.UUID, _ uuid.UUID, status string, _ string) error {
	s.markOperationFailedCalled = true
	s.markOperationFailedStatus = status
	return nil
}
func (s *memoryManagedResourceStore) Load(context.Context, ManagedResourceAuthContext, uuid.UUID) (*LoadedManagedResource, error) {
	return s.loaded, nil
}
func (s *memoryManagedResourceStore) LoadForOperation(context.Context, ManagedResourceAuthContext, uuid.UUID, uuid.UUID) (*LoadedManagedResource, error) {
	return s.loaded, nil
}
func (s *memoryManagedResourceStore) SaveState(ctx context.Context, auth ManagedResourceAuthContext, input SaveManagedResourceStateInput) error {
	s.saveCalled = true
	s.savedInput = input
	return nil
}
func (s *memoryManagedResourceStore) SaveRefreshedState(context.Context, ManagedResourceAuthContext, SaveManagedResourceStateInput) error {
	s.saveRefreshedCalled = true
	return nil
}
func (s *memoryManagedResourceStore) RecordMissing(context.Context, ManagedResourceAuthContext, uuid.UUID, uuid.UUID, int, map[string]any) (string, error) {
	return models.ManagedResourceEventMissing, nil
}
func (s *memoryManagedResourceStore) Delete(context.Context, ManagedResourceAuthContext, uuid.UUID, uuid.UUID) error {
	s.deleteCalled = true
	return nil
}
func (s *memoryManagedResourceStore) ForceForget(context.Context, ManagedResourceAuthContext, uuid.UUID, uuid.UUID) error {
	s.forceForgetCalled = true
	return nil
}
