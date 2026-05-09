package terraform

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/database"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestRecordManagedResourceEventStoresSanitizedOutputsAndHashInput(t *testing.T) {
	db := testManagedResourceStoreDB(t)
	fixture := managedResourceStoreFixture(t, db)
	resource := createEventTestManagedResource(t, fixture)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := RecordManagedResourceEvent(
			tx,
			resource.ManagedResourceID,
			models.ManagedResourceEventCreated,
			map[string]any{"password": "$terraformIntegrationSecret:event-1"},
			map[string]any{"password": "$sensitive"},
			testStringPtr("hash"),
			map[string]any{"source": "test"},
		)
		return err
	})
	require.NoError(t, err)

	events, err := models.ListPendingManagedResourceEvents(10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "$terraformIntegrationSecret:event-1", events[0].Outputs.Data()["password"])
	assert.Equal(t, "$sensitive", events[0].HashInput.Data()["password"])
	assert.NotContains(t, events[0].Outputs.Data(), "raw-secret")
}

func TestDetectAndRecordManagedResourceChangeUsesStableHashInput(t *testing.T) {
	db := testManagedResourceStoreDB(t)
	fixture := managedResourceStoreFixture(t, db)
	resource := createEventTestManagedResource(t, fixture)

	hashInput := map[string]any{"id": "remote-1", "password": "$sensitive"}
	previousHash, err := ComputeOutputsHash(hashInput, nil)
	require.NoError(t, err)

	err = db.Transaction(func(tx *gorm.DB) error {
		_, _, changed, err := DetectAndRecordManagedResourceChange(
			tx,
			resource,
			&previousHash,
			map[string]any{"id": "remote-1", "password": "$terraformIntegrationSecret:event-2"},
			hashInput,
			nil,
			map[string]any{"source": "test"},
		)
		require.NoError(t, err)
		assert.False(t, changed)
		return nil
	})
	require.NoError(t, err)

	events, err := models.ListPendingManagedResourceEvents(10)
	require.NoError(t, err)
	assert.Empty(t, events)

	err = db.Transaction(func(tx *gorm.DB) error {
		_, _, changed, err := DetectAndRecordManagedResourceChange(
			tx,
			resource,
			&previousHash,
			map[string]any{"id": "remote-2", "password": "$terraformIntegrationSecret:event-3"},
			map[string]any{"id": "remote-2", "password": "$sensitive"},
			[]string{"id"},
			map[string]any{"source": "test"},
		)
		require.NoError(t, err)
		assert.True(t, changed)
		return nil
	})
	require.NoError(t, err)

	events, err = models.ListPendingManagedResourceEvents(10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "remote-2", events[0].Outputs.Data()["id"])
}

func createEventTestManagedResource(t *testing.T, fixture managedResourceFixture) models.TerraformManagedResource {
	t.Helper()
	now := time.Now()
	resource := models.TerraformManagedResource{
		ManagedResourceID: uuid.New(),
		OrganizationID:    fixture.OrganizationID,
		CanvasID:          fixture.CanvasID,
		IntegrationID:     fixture.IntegrationID,
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
	require.NoError(t, database.Conn().Create(&resource).Error)
	return resource
}
