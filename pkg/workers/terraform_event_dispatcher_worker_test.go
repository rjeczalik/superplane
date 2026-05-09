package workers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/database"
	terraformintegration "github.com/superplanehq/superplane/pkg/integrations/terraform"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestTerraformEventDispatcherWorkerFansOutPendingEvent(t *testing.T) {
	db := terraformEventDispatcherTestDB(t)
	fixture := terraformEventDispatcherFixture(t, db)
	_, err := models.UpsertSubscriptionForNode(db, fixture.organizationID, fixture.canvasID, fixture.integrationID, "trigger-1", fixture.resource.ResourceType, models.TerraformManagedResourceSubscriptionConfig{
		ChangedFields:    []string{"id"},
		PollIntervalSecs: 300,
		Enabled:          true,
	})
	require.NoError(t, err)

	outputsHash, err := terraformintegration.ComputeOutputsHash(map[string]any{"id": "remote-1"}, []string{"id"})
	require.NoError(t, err)
	event, err := models.CreateManagedResourceEvent(db, fixture.resource.ManagedResourceID, models.ManagedResourceEventUpdated, map[string]any{"id": "remote-1", "password": "$terraformIntegrationSecret:event-1"}, map[string]any{"id": "remote-1", "password": "$sensitive"}, &outputsHash, map[string]any{"source": "test"})
	require.NoError(t, err)

	worker := NewTerraformEventDispatcherWorker()
	var published []uuid.UUID
	worker.publish = func(event *models.CanvasEvent) error {
		published = append(published, event.ID)
		return nil
	}

	created, err := worker.LockAndProcessEvent(event.ID)
	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, "trigger-1", created[0].NodeID)
	assert.Equal(t, "default", created[0].Channel)
	assert.Equal(t, models.CanvasEventStatePending, created[0].State)
	require.Len(t, published, 1)

	var workflowEvent models.CanvasEvent
	require.NoError(t, db.Where("id = ?", created[0].ID).First(&workflowEvent).Error)
	payload := workflowEvent.Data.Data().(map[string]any)
	outputs := payload["outputs"].(map[string]any)
	assert.Equal(t, "$terraformIntegrationSecret:event-1", outputs["password"])

	var reloaded models.TerraformManagedResourceEvent
	require.NoError(t, db.Where("id = ?", event.ID).First(&reloaded).Error)
	assert.Equal(t, models.ManagedResourceEventStateProcessed, reloaded.State)
}

func TestTerraformEventDispatcherWorkerSkipsUnchangedSubscriptionCursor(t *testing.T) {
	db := terraformEventDispatcherTestDB(t)
	fixture := terraformEventDispatcherFixture(t, db)
	subscription, err := models.UpsertSubscriptionForNode(db, fixture.organizationID, fixture.canvasID, fixture.integrationID, "trigger-1", fixture.resource.ResourceType, models.TerraformManagedResourceSubscriptionConfig{
		ChangedFields:    []string{"id"},
		PollIntervalSecs: 300,
		Enabled:          true,
	})
	require.NoError(t, err)

	outputsHash, err := terraformintegration.ComputeOutputsHash(map[string]any{"id": "remote-1"}, []string{"id"})
	require.NoError(t, err)
	event, err := models.CreateManagedResourceEvent(db, fixture.resource.ManagedResourceID, models.ManagedResourceEventUpdated, map[string]any{"id": "remote-1", "password": "$terraformIntegrationSecret:event-2"}, map[string]any{"id": "remote-1", "password": "$sensitive"}, &outputsHash, map[string]any{"source": "test"})
	require.NoError(t, err)

	cursor, err := models.FindOrCreateManagedResourceSubscriptionCursor(db, subscription.ID, fixture.resource.ManagedResourceID)
	require.NoError(t, err)
	require.NoError(t, cursor.Update(db, &outputsHash, event.ID))

	worker := NewTerraformEventDispatcherWorker()
	worker.publish = func(event *models.CanvasEvent) error {
		t.Fatalf("publish should not be called for unchanged cursor")
		return nil
	}
	created, err := worker.LockAndProcessEvent(event.ID)
	require.NoError(t, err)
	assert.Empty(t, created)
}

func TestTerraformEventDispatcherWorkerDispatchesChangedEventTypeWithSameHash(t *testing.T) {
	db := terraformEventDispatcherTestDB(t)
	fixture := terraformEventDispatcherFixture(t, db)
	subscription, err := models.UpsertSubscriptionForNode(db, fixture.organizationID, fixture.canvasID, fixture.integrationID, "trigger-1", fixture.resource.ResourceType, models.TerraformManagedResourceSubscriptionConfig{
		ChangedFields:    []string{"id"},
		PollIntervalSecs: 300,
		Enabled:          true,
	})
	require.NoError(t, err)

	outputsHash, err := terraformintegration.ComputeOutputsHash(map[string]any{}, []string{"id"})
	require.NoError(t, err)
	previous, err := models.CreateManagedResourceEvent(db, fixture.resource.ManagedResourceID, models.ManagedResourceEventMissing, map[string]any{}, map[string]any{}, &outputsHash, map[string]any{"source": "test"})
	require.NoError(t, err)
	cursor, err := models.FindOrCreateManagedResourceSubscriptionCursor(db, subscription.ID, fixture.resource.ManagedResourceID)
	require.NoError(t, err)
	require.NoError(t, cursor.Update(db, &outputsHash, previous.ID))

	deleted, err := models.CreateManagedResourceEvent(db, fixture.resource.ManagedResourceID, models.ManagedResourceEventExternallyDeleted, map[string]any{}, map[string]any{}, &outputsHash, map[string]any{"source": "test"})
	require.NoError(t, err)

	worker := NewTerraformEventDispatcherWorker()
	worker.publish = func(event *models.CanvasEvent) error { return nil }
	created, err := worker.LockAndProcessEvent(deleted.ID)
	require.NoError(t, err)
	require.Len(t, created, 1)
	payload := created[0].Data.Data().(map[string]any)
	assert.Equal(t, models.ManagedResourceEventExternallyDeleted, payload["event_type"])
}

type terraformEventDispatcherFixtureData struct {
	organizationID uuid.UUID
	canvasID       uuid.UUID
	integrationID  uuid.UUID
	resource       models.TerraformManagedResource
}

func terraformEventDispatcherTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NoError(t, database.TruncateTables())
	return database.Conn()
}

func terraformEventDispatcherFixture(t *testing.T, db *gorm.DB) terraformEventDispatcherFixtureData {
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
		NodeID:            "trigger-1",
		Name:              "trigger",
		Type:              "trigger",
		State:             "ready",
		Ref:               datatypes.NewJSONType(models.NodeRef{}),
		Configuration:     datatypes.NewJSONType(map[string]any{}),
		AppInstallationID: &integration.ID,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	require.NoError(t, db.Create(&node).Error)

	resource := models.TerraformManagedResource{
		ManagedResourceID: uuid.New(),
		OrganizationID:    org.ID,
		CanvasID:          canvas.ID,
		IntegrationID:     integration.ID,
		ProviderName:      "talos",
		ProviderSource:    "registry.terraform.io/siderolabs/talos",
		ProviderVersion:   "0.11.0",
		ResourceType:      "talos_machine_secrets",
		Status:            models.ManagedResourceStatusReady,
		Health:            models.ManagedResourceHealthHealthy,
		RetentionPolicy:   datatypes.NewJSONType(map[string]any{}),
		RecoveryHints:     datatypes.NewJSONType(map[string]any{}),
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	require.NoError(t, db.Create(&resource).Error)

	return terraformEventDispatcherFixtureData{
		organizationID: org.ID,
		canvasID:       canvas.ID,
		integrationID:  integration.ID,
		resource:       resource,
	}
}
