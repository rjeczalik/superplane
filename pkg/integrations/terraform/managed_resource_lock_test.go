package terraform

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTryAdvisoryLockForIdempotency(t *testing.T) {
	db := testManagedResourceStoreDB(t)
	canvasID := uuid.New()
	integrationID := uuid.New()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		acquired, err := TryAdvisoryLockForIdempotency(tx, canvasID, integrationID, "vultr_instance", "key-1")
		require.NoError(t, err)
		require.True(t, acquired)
		return nil
	}))
}

func TestTryAdvisoryLockForIdempotencyConflictsAcrossTransactions(t *testing.T) {
	db := testManagedResourceStoreDB(t)
	canvasID := uuid.New()
	integrationID := uuid.New()

	txA := db.Begin()
	require.NoError(t, txA.Error)
	defer txA.Rollback()

	acquired, err := TryAdvisoryLockForIdempotency(txA, canvasID, integrationID, "vultr_instance", "key-1")
	require.NoError(t, err)
	require.True(t, acquired)

	txB := db.Begin()
	require.NoError(t, txB.Error)
	defer txB.Rollback()

	acquired, err = TryAdvisoryLockForIdempotency(txB, canvasID, integrationID, "vultr_instance", "key-1")
	require.NoError(t, err)
	require.False(t, acquired)

	acquired, err = TryAdvisoryLockForIdempotency(txB, canvasID, integrationID, "vultr_instance", "key-2")
	require.NoError(t, err)
	require.True(t, acquired)
}
