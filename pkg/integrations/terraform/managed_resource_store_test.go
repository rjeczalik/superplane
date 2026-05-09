package terraform

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/database"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestManagedResourceStoreBeginCreateAndCompleteCreate(t *testing.T) {
	db := testManagedResourceStoreDB(t)
	store := NewGormManagedResourceStore(db, TerraformStateEncryptorsFromLegacy(&recordingEncryptor{}))
	fixture := managedResourceStoreFixture(t, db)
	auth := ManagedResourceSystemAuth(fixture.OrganizationID, fixture.CanvasID, fixture.IntegrationID, "workflow_execution")

	operationID := uuid.New()
	resource, err := store.BeginCreate(context.Background(), auth, BeginManagedResourceCreateInput{
		ManagedResourceID:    uuid.New(),
		OperationID:          operationID,
		OrganizationID:       fixture.OrganizationID,
		CanvasID:             fixture.CanvasID,
		IntegrationID:        fixture.IntegrationID,
		CreatedByNodeID:      "node-1",
		ProviderName:         "talos",
		ProviderSource:       "registry.terraform.io/siderolabs/talos",
		ProviderVersion:      "0.11.0",
		ResourceType:         "talos_machine_secrets",
		IdempotencyKey:       testStringPtr("same"),
		OperationLeaseUntil:  time.Now().Add(time.Hour),
		CreatedByExecutionID: fixture.ExecutionID,
		CreatedByEventID:     fixture.EventID,
		RootEventID:          fixture.EventID,
	})
	require.NoError(t, err)
	require.Equal(t, models.ManagedResourceStatusCreating, resource.Status)
	require.Equal(t, operationID, *resource.CurrentOperationID)

	err = store.CompleteCreate(context.Background(), auth, CompleteManagedResourceCreateInput{
		ManagedResourceID: resource.ManagedResourceID,
		OperationID:       operationID,
		StatePayload:      []byte(`{"state":1}`),
		ConfigPayload:     []byte(`{"config":1}`),
		SchemaHash:        "hash",
		StateFormat:       TerraformStateFormatRuntime,
		RemoteID:          testStringPtr("remote-1"),
		DisplayName:       testStringPtr("machine secrets"),
		SanitizedOutputs:  map[string]any{"id": "remote-1"},
		HashInput:         map[string]any{"id": "remote-1"},
		OutputsHash:       testStringPtr("outputs-hash"),
		EventMetadata:     map[string]any{"source": "test"},
	})
	require.NoError(t, err)

	loaded, err := store.Load(context.Background(), auth, resource.ManagedResourceID)
	require.NoError(t, err)
	require.Equal(t, models.ManagedResourceStatusReady, loaded.Resource.Status)
	require.Equal(t, []byte(`{"state":1}`), loaded.StatePayload)
	require.Equal(t, []byte(`{"config":1}`), loaded.ConfigPayload)
	require.Nil(t, loaded.Resource.CurrentOperationID)

	events, err := models.ListPendingManagedResourceEvents(10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, models.ManagedResourceEventCreated, events[0].EventType)
}

func TestManagedResourceStoreRejectsMissingAuth(t *testing.T) {
	store := NewGormManagedResourceStore(testManagedResourceStoreDB(t), TerraformStateEncryptorsFromLegacy(&recordingEncryptor{}))
	_, err := store.BeginCreate(context.Background(), ManagedResourceAuthContext{}, BeginManagedResourceCreateInput{})
	require.ErrorIs(t, err, ErrManagedResourceUnauthorized)
}

type managedResourceFixture struct {
	OrganizationID uuid.UUID
	CanvasID       uuid.UUID
	IntegrationID  uuid.UUID
	EventID        *uuid.UUID
	ExecutionID    *uuid.UUID
}

func testManagedResourceStoreDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NoError(t, database.TruncateTables())
	return database.Conn()
}

func managedResourceStoreFixture(t *testing.T, db *gorm.DB) managedResourceFixture {
	t.Helper()
	now := time.Now()
	org := models.Organization{ID: uuid.New(), Name: "org-" + uuid.NewString()}
	require.NoError(t, db.Create(&org).Error)

	liveVersionID := uuid.New()
	canvas := models.Canvas{ID: uuid.New(), OrganizationID: org.ID, LiveVersionID: &liveVersionID, Name: "canvas-" + uuid.NewString(), CreatedAt: &now, UpdatedAt: &now}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&canvas).Error; err != nil {
			return err
		}
		return tx.Create(&models.CanvasVersion{
			ID:          liveVersionID,
			WorkflowID:  canvas.ID,
			State:       models.CanvasVersionStatePublished,
			PublishedAt: &now,
			Nodes:       datatypes.NewJSONSlice([]models.Node{}),
			Edges:       datatypes.NewJSONSlice([]models.Edge{}),
			CreatedAt:   &now,
			UpdatedAt:   &now,
		}).Error
	}))

	integration, err := models.CreateIntegration(uuid.New(), org.ID, "terraform", "terraform", map[string]any{})
	require.NoError(t, err)

	node := models.CanvasNode{
		WorkflowID:        canvas.ID,
		NodeID:            "node-1",
		Name:              "node",
		Type:              "action",
		State:             "ready",
		Ref:               datatypes.NewJSONType(models.NodeRef{}),
		Configuration:     datatypes.NewJSONType(map[string]any{}),
		AppInstallationID: &integration.ID,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	require.NoError(t, db.Create(&node).Error)

	event := models.CanvasEvent{ID: uuid.New(), WorkflowID: canvas.ID, NodeID: "node-1", State: models.CanvasEventStateRouted, Data: datatypes.NewJSONType(any(map[string]any{})), CreatedAt: &now}
	require.NoError(t, db.Create(&event).Error)

	execution := models.CanvasNodeExecution{ID: uuid.New(), WorkflowID: canvas.ID, NodeID: "node-1", RootEventID: event.ID, EventID: event.ID, State: models.CanvasNodeExecutionStatePending, Configuration: datatypes.NewJSONType(map[string]any{}), CreatedAt: &now, UpdatedAt: &now}
	require.NoError(t, db.Create(&execution).Error)

	return managedResourceFixture{
		OrganizationID: org.ID,
		CanvasID:       canvas.ID,
		IntegrationID:  integration.ID,
		EventID:        &event.ID,
		ExecutionID:    &execution.ID,
	}
}

func testStringPtr(v string) *string {
	return &v
}
