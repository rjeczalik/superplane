package workers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/database"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestTerraformEventRetentionWorkerDeletesProcessedEventsAndExpiredDeletedMetadata(t *testing.T) {
	db := terraformEventRetentionTestDB(t)
	fixture := terraformEventRetentionFixture(t, db)
	processedEvent := createRetentionEvent(t, db, fixture.deletedResource.ManagedResourceID, models.ManagedResourceEventStateProcessed, time.Now().Add(-48*time.Hour))
	pendingEvent := createRetentionEvent(t, db, fixture.pendingDeletedResource.ManagedResourceID, models.ManagedResourceEventStatePending, time.Now().Add(-48*time.Hour))

	worker := NewTerraformEventRetentionWorker()
	require.NoError(t, worker.Cleanup(time.Now()))

	assertRecordMissing(t, db, &models.TerraformManagedResourceEvent{}, processedEvent.ID)
	assertRecordExists(t, db, &models.TerraformManagedResourceEvent{}, pendingEvent.ID)
	assertRecordMissing(t, db.Unscoped(), &models.TerraformManagedResource{}, fixture.deletedResource.ID)
	assertRecordExists(t, db.Unscoped(), &models.TerraformManagedResource{}, fixture.activeResource.ID)
	assertRecordExists(t, db.Unscoped(), &models.TerraformManagedResource{}, fixture.pendingDeletedResource.ID)
}

type terraformEventRetentionFixtureData struct {
	deletedResource        models.TerraformManagedResource
	pendingDeletedResource models.TerraformManagedResource
	activeResource         models.TerraformManagedResource
}

func terraformEventRetentionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NoError(t, database.TruncateTables())
	return database.Conn()
}

func terraformEventRetentionFixture(t *testing.T, db *gorm.DB) terraformEventRetentionFixtureData {
	t.Helper()
	now := time.Now()
	retentionDays := int32(1)
	org := models.Organization{ID: uuid.New(), Name: "org-" + uuid.NewString(), UsageRetentionWindowDays: &retentionDays}
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

	deletedAt := now.Add(-48 * time.Hour)
	deleted := retentionResource(org.ID, canvas.ID, integration.ID, models.ManagedResourceStatusDeleted, &deletedAt)
	pendingDeleted := retentionResource(org.ID, canvas.ID, integration.ID, models.ManagedResourceStatusDeleted, &deletedAt)
	active := retentionResource(org.ID, canvas.ID, integration.ID, models.ManagedResourceStatusReady, nil)
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Create(&pendingDeleted).Error)
	require.NoError(t, db.Create(&active).Error)

	return terraformEventRetentionFixtureData{
		deletedResource:        deleted,
		pendingDeletedResource: pendingDeleted,
		activeResource:         active,
	}
}

func retentionResource(orgID, canvasID, integrationID uuid.UUID, status string, deletedAt *time.Time) models.TerraformManagedResource {
	now := time.Now()
	resource := models.TerraformManagedResource{
		ID:                uuid.New(),
		ManagedResourceID: uuid.New(),
		OrganizationID:    orgID,
		CanvasID:          canvasID,
		IntegrationID:     integrationID,
		ProviderName:      "talos",
		ProviderSource:    "registry.terraform.io/siderolabs/talos",
		ProviderVersion:   "0.11.0",
		ResourceType:      "talos_machine_secrets",
		Status:            status,
		Health:            models.ManagedResourceHealthHealthy,
		RetentionPolicy:   datatypes.NewJSONType(map[string]any{}),
		RecoveryHints:     datatypes.NewJSONType(map[string]any{}),
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	if deletedAt != nil {
		resource.DeletedAt = gorm.DeletedAt{Time: *deletedAt, Valid: true}
	}
	return resource
}

func createRetentionEvent(t *testing.T, db *gorm.DB, resourceID uuid.UUID, state string, createdAt time.Time) models.TerraformManagedResourceEvent {
	t.Helper()
	var resource models.TerraformManagedResource
	require.NoError(t, db.Unscoped().Where("managed_resource_id = ?", resourceID).First(&resource).Error)
	now := time.Now()
	event := models.TerraformManagedResourceEvent{
		ID:                uuid.New(),
		ManagedResourceID: resource.ManagedResourceID,
		OrganizationID:    resource.OrganizationID,
		CanvasID:          resource.CanvasID,
		IntegrationID:     resource.IntegrationID,
		ResourceType:      resource.ResourceType,
		EventType:         models.ManagedResourceEventUpdated,
		State:             state,
		OutputsHash:       stringPtr("hash"),
		Outputs:           datatypes.NewJSONType(map[string]any{"id": "remote"}),
		HashInput:         datatypes.NewJSONType(map[string]any{"id": "remote"}),
		Metadata:          datatypes.NewJSONType(map[string]any{}),
		CreatedAt:         &createdAt,
		UpdatedAt:         &now,
	}
	require.NoError(t, db.Create(&event).Error)
	return event
}

func stringPtr(v string) *string { return &v }

func assertRecordMissing(t *testing.T, db *gorm.DB, dest any, id uuid.UUID) {
	t.Helper()
	err := db.Where("id = ?", id).First(dest).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func assertRecordExists(t *testing.T, db *gorm.DB, dest any, id uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Where("id = ?", id).First(dest).Error)
}
