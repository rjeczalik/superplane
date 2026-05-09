package terraform

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/authorization"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ManagedResourceAuthModeUser   = "user"
	ManagedResourceAuthModeSystem = "system"
)

var (
	ErrManagedResourceUnauthorized = errors.New("managed resource unauthorized")
	ErrManagedResourceConflict     = errors.New("managed resource conflict")
	ErrManagedResourceUnsupported  = errors.New("managed resource operation not implemented")
	ErrLockMismatch                = errors.New("terraform state lock mismatch")
)

type ManagedResourceAuthContext struct {
	Mode           string
	OrganizationID uuid.UUID
	CanvasID       uuid.UUID
	IntegrationID  uuid.UUID
	UserID         uuid.UUID
	Authorization  authorization.Authorization
	SystemReason   string
}

func ManagedResourceSystemAuth(orgID, canvasID, integrationID uuid.UUID, reason string) ManagedResourceAuthContext {
	return ManagedResourceAuthContext{
		Mode:           ManagedResourceAuthModeSystem,
		OrganizationID: orgID,
		CanvasID:       canvasID,
		IntegrationID:  integrationID,
		SystemReason:   reason,
	}
}

type BeginManagedResourceCreateInput struct {
	ManagedResourceID    uuid.UUID
	OperationID          uuid.UUID
	OrganizationID       uuid.UUID
	CanvasID             uuid.UUID
	IntegrationID        uuid.UUID
	CreatedByNodeID      string
	CreatedByExecutionID *uuid.UUID
	CreatedByEventID     *uuid.UUID
	RootEventID          *uuid.UUID
	ProviderName         string
	ProviderSource       string
	ProviderVersion      string
	ResourceType         string
	IdempotencyKey       *string
	OperationLeaseUntil  time.Time
	RetentionPolicy      map[string]any
}

type CompleteManagedResourceCreateInput struct {
	ManagedResourceID uuid.UUID
	OperationID       uuid.UUID
	StatePayload      []byte
	ConfigPayload     []byte
	SchemaHash        string
	StateFormat       string
	RemoteID          *string
	DisplayName       *string
	SanitizedOutputs  map[string]any
	HashInput         map[string]any
	OutputsHash       *string
	EventMetadata     map[string]any
}

type LoadedManagedResource struct {
	Resource      models.TerraformManagedResource
	State         models.TerraformManagedResourceState
	StatePayload  []byte
	ConfigPayload []byte
}

type ManagedResourceStore interface {
	BeginCreate(ctx context.Context, auth ManagedResourceAuthContext, input BeginManagedResourceCreateInput) (*models.TerraformManagedResource, error)
	CompleteCreate(ctx context.Context, auth ManagedResourceAuthContext, input CompleteManagedResourceCreateInput) error
	MarkCreateProviderFailed(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, message string) error
	MarkCreateOrphanRisk(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, message string, recoveryHints map[string]any) error
	FindExistingForIdempotency(ctx context.Context, auth ManagedResourceAuthContext, resourceType, key string) (*models.TerraformManagedResource, error)
	ClaimOperation(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID uuid.UUID, operation string, leaseUntil time.Time, allowedStatuses []string) (uuid.UUID, error)
	RefreshOperationLease(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, leaseUntil time.Time) error
	MarkOperationFailed(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, status string, message string) error
	Load(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID uuid.UUID) (*LoadedManagedResource, error)
	LoadForOperation(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID) (*LoadedManagedResource, error)
	SaveState(ctx context.Context, auth ManagedResourceAuthContext, input SaveManagedResourceStateInput) error
	SaveRefreshedState(ctx context.Context, auth ManagedResourceAuthContext, input SaveManagedResourceStateInput) error
	RecordMissing(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, threshold int, metadata map[string]any) (string, error)
	Delete(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID) error
	ForceForget(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID) error
}

type SaveManagedResourceStateInput struct {
	ManagedResourceID   uuid.UUID
	OperationID         uuid.UUID
	ExpectedLockVersion int64
	StatePayload        []byte
	ConfigPayload       []byte
	SchemaHash          string
	StateFormat         string
	SanitizedOutputs    map[string]any
	HashInput           map[string]any
	OutputsHash         *string
	EventMetadata       map[string]any
	EventType           string
	KeepOperationLease  bool
}

type GormManagedResourceStore struct {
	db         *gorm.DB
	encryptors TerraformStateEncryptors
}

func NewGormManagedResourceStore(db *gorm.DB, encryptors TerraformStateEncryptors) *GormManagedResourceStore {
	return &GormManagedResourceStore{db: db, encryptors: encryptors}
}

func (s *GormManagedResourceStore) BeginCreate(ctx context.Context, auth ManagedResourceAuthContext, input BeginManagedResourceCreateInput) (*models.TerraformManagedResource, error) {
	if input.ManagedResourceID == uuid.Nil {
		input.ManagedResourceID = uuid.New()
	}
	if input.OperationID == uuid.Nil {
		input.OperationID = uuid.New()
	}
	if input.OperationLeaseUntil.IsZero() {
		input.OperationLeaseUntil = time.Now().Add(30 * time.Minute)
	}
	if err := s.authorize(auth, "create"); err != nil {
		return nil, err
	}
	if err := validateAuthScope(auth, input.OrganizationID, input.CanvasID, input.IntegrationID); err != nil {
		return nil, err
	}

	var created models.TerraformManagedResource
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockActiveParents(tx, input.OrganizationID, input.CanvasID, input.IntegrationID); err != nil {
			return err
		}
		if input.IdempotencyKey != nil {
			acquired, err := TryAdvisoryLockForIdempotency(tx, input.CanvasID, input.IntegrationID, input.ResourceType, *input.IdempotencyKey)
			if err != nil {
				return err
			}
			if !acquired {
				return ErrManagedResourceConflict
			}
		}

		now := time.Now()
		resource := models.TerraformManagedResource{
			ManagedResourceID:    input.ManagedResourceID,
			OrganizationID:       input.OrganizationID,
			IntegrationID:        input.IntegrationID,
			CanvasID:             input.CanvasID,
			CreatedByNodeID:      input.CreatedByNodeID,
			CreatedByExecutionID: input.CreatedByExecutionID,
			CreatedByEventID:     input.CreatedByEventID,
			RootEventID:          input.RootEventID,
			ProviderName:         input.ProviderName,
			ProviderSource:       input.ProviderSource,
			ProviderVersion:      input.ProviderVersion,
			ResourceType:         input.ResourceType,
			IdempotencyKey:       input.IdempotencyKey,
			Status:               models.ManagedResourceStatusCreating,
			Health:               models.ManagedResourceHealthHealthy,
			LastOperation:        managedResourceStringPtr("create"),
			RetentionPolicy:      datatypes.NewJSONType(nilToEmptyMap(input.RetentionPolicy)),
			RecoveryHints:        datatypes.NewJSONType(map[string]any{}),
			CurrentOperationID:   &input.OperationID,
			OperationStartedAt:   &now,
			OperationExpiresAt:   &input.OperationLeaseUntil,
			CreatedAt:            &now,
			UpdatedAt:            &now,
		}
		if err := tx.Create(&resource).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrManagedResourceConflict
			}
			return err
		}
		created = resource
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (s *GormManagedResourceStore) CompleteCreate(ctx context.Context, auth ManagedResourceAuthContext, input CompleteManagedResourceCreateInput) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.LockManagedResource(tx, input.ManagedResourceID)
		if err != nil {
			return err
		}
		if err := s.authorizeResource(auth, resource, "create"); err != nil {
			return err
		}
		if resource.CurrentOperationID == nil || *resource.CurrentOperationID != input.OperationID {
			return models.ErrManagedResourceOperationInProgress
		}

		stateNonce, err := randomNonce()
		if err != nil {
			return err
		}
		configNonce, err := randomNonce()
		if err != nil {
			return err
		}
		stateCiphertext, err := s.encryptors.Encrypt(ctx, input.StatePayload, managedResourceStateAD(*resource, stateNonce, input.StateFormat), TerraformStateEncryptionV2)
		if err != nil {
			return err
		}
		configCiphertext, err := s.encryptors.Encrypt(ctx, input.ConfigPayload, managedResourceConfigAD(*resource, configNonce, input.StateFormat), TerraformStateEncryptionV2)
		if err != nil {
			return err
		}

		now := time.Now()
		state := models.TerraformManagedResourceState{
			ManagedResourceID:    resource.ManagedResourceID,
			StateCiphertext:      stateCiphertext,
			StateNonce:           stateNonce,
			LastConfigCiphertext: configCiphertext,
			LastConfigNonce:      configNonce,
			SchemaHash:           input.SchemaHash,
			EncryptionVersion:    2,
			StateFormat:          input.StateFormat,
			LockVersion:          0,
			CreatedAt:            &now,
			UpdatedAt:            &now,
		}
		if err := tx.Create(&state).Error; err != nil {
			return err
		}
		if _, err := models.CreateManagedResourceEvent(tx, resource.ManagedResourceID, models.ManagedResourceEventCreated, input.SanitizedOutputs, input.HashInput, input.OutputsHash, input.EventMetadata); err != nil {
			return err
		}
		return models.CompleteManagedResourceOperation(tx, resource.ManagedResourceID, input.OperationID, map[string]any{
			"status":            models.ManagedResourceStatusReady,
			"health":            models.ManagedResourceHealthHealthy,
			"remote_id":         input.RemoteID,
			"display_name":      input.DisplayName,
			"last_refreshed_at": now,
			"orphan_risk":       false,
		})
	})
}

func (s *GormManagedResourceStore) Load(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID uuid.UUID) (*LoadedManagedResource, error) {
	var loaded LoadedManagedResource
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.FindManagedResourceInTransaction(tx, managedResourceID)
		if err != nil {
			return err
		}
		if err := s.authorizeResource(auth, resource, "read"); err != nil {
			return err
		}
		state, err := models.FindManagedResourceState(tx, managedResourceID)
		if err != nil {
			return err
		}
		statePayload, err := s.encryptors.Decrypt(ctx, state.StateCiphertext, managedResourceStateAD(*resource, state.StateNonce, state.StateFormat), TerraformStateEncryptionV2)
		if err != nil {
			return err
		}
		configPayload, err := s.encryptors.Decrypt(ctx, state.LastConfigCiphertext, managedResourceConfigAD(*resource, state.LastConfigNonce, state.StateFormat), TerraformStateEncryptionV2)
		if err != nil {
			return err
		}
		loaded = LoadedManagedResource{Resource: *resource, State: *state, StatePayload: statePayload, ConfigPayload: configPayload}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &loaded, nil
}

func (s *GormManagedResourceStore) authorizeResource(auth ManagedResourceAuthContext, resource *models.TerraformManagedResource, action string) error {
	if err := s.authorize(auth, action); err != nil {
		return err
	}
	return validateAuthScope(auth, resource.OrganizationID, resource.CanvasID, resource.IntegrationID)
}

func (s *GormManagedResourceStore) authorize(auth ManagedResourceAuthContext, action string) error {
	switch auth.Mode {
	case ManagedResourceAuthModeSystem:
		if auth.OrganizationID == uuid.Nil || auth.CanvasID == uuid.Nil || auth.IntegrationID == uuid.Nil || auth.SystemReason == "" {
			return ErrManagedResourceUnauthorized
		}
		return nil
	case ManagedResourceAuthModeUser:
		if auth.OrganizationID == uuid.Nil || auth.UserID == uuid.Nil || auth.Authorization == nil {
			return ErrManagedResourceUnauthorized
		}
		allowed, err := auth.Authorization.CheckOrganizationPermission(auth.UserID.String(), auth.OrganizationID.String(), "managed_resources", action)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrManagedResourceUnauthorized
		}
		return nil
	default:
		return ErrManagedResourceUnauthorized
	}
}

func validateAuthScope(auth ManagedResourceAuthContext, orgID, canvasID, integrationID uuid.UUID) error {
	if auth.OrganizationID != orgID || auth.CanvasID != canvasID || auth.IntegrationID != integrationID {
		return ErrManagedResourceUnauthorized
	}
	return nil
}

func lockActiveParents(tx *gorm.DB, orgID, canvasID, integrationID uuid.UUID) error {
	var org models.Organization
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orgID).Where("deleted_at IS NULL").First(&org).Error; err != nil {
		return err
	}
	var canvas models.Canvas
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", canvasID).Where("organization_id = ?", orgID).Where("deleted_at IS NULL").First(&canvas).Error; err != nil {
		return err
	}
	var integration models.Integration
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", integrationID).Where("organization_id = ?", orgID).Where("deleted_at IS NULL").First(&integration).Error
}

func managedResourceStateAD(resource models.TerraformManagedResource, nonce []byte, stateFormat string) []byte {
	return managedResourceAssociatedData(
		nonce,
		resource.OrganizationID,
		resource.IntegrationID,
		resource.CanvasID,
		resource.ManagedResourceID,
		resource.ProviderName,
		resource.ProviderSource,
		resource.ProviderVersion,
		resource.ResourceType,
		stateFormat,
		"state",
	)
}

func managedResourceConfigAD(resource models.TerraformManagedResource, nonce []byte, stateFormat string) []byte {
	return managedResourceAssociatedData(
		nonce,
		resource.OrganizationID,
		resource.IntegrationID,
		resource.CanvasID,
		resource.ManagedResourceID,
		resource.ProviderName,
		resource.ProviderSource,
		resource.ProviderVersion,
		resource.ResourceType,
		stateFormat,
		"config",
	)
}

func nilToEmptyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func managedResourceStringPtr(value string) *string {
	return &value
}

func (s *GormManagedResourceStore) FindExistingForIdempotency(ctx context.Context, auth ManagedResourceAuthContext, resourceType, key string) (*models.TerraformManagedResource, error) {
	if err := s.authorize(auth, "read"); err != nil {
		return nil, err
	}
	resource, err := models.LockManagedResourceForIdempotency(s.db.WithContext(ctx), auth.CanvasID, auth.IntegrationID, resourceType, key)
	if err != nil {
		return nil, err
	}
	if err := validateAuthScope(auth, resource.OrganizationID, resource.CanvasID, resource.IntegrationID); err != nil {
		return nil, err
	}
	return resource, nil
}

func (s *GormManagedResourceStore) MarkCreateProviderFailed(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, message string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.LockManagedResource(tx, managedResourceID)
		if err != nil {
			return err
		}
		if err := s.authorizeResource(auth, resource, "create"); err != nil {
			return err
		}
		now := time.Now()
		return models.CompleteManagedResourceOperation(tx, managedResourceID, operationID, map[string]any{
			"status":        models.ManagedResourceStatusDeleted,
			"deleted_at":    now,
			"last_error":    message,
			"last_error_at": now,
		})
	})
}

func (s *GormManagedResourceStore) MarkCreateOrphanRisk(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, message string, recoveryHints map[string]any) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.LockManagedResource(tx, managedResourceID)
		if err != nil {
			return err
		}
		if err := s.authorizeResource(auth, resource, "create"); err != nil {
			return err
		}
		now := time.Now()
		return models.CompleteManagedResourceOperation(tx, managedResourceID, operationID, map[string]any{
			"status":         models.ManagedResourceStatusCreating,
			"orphan_risk":    true,
			"recovery_hints": datatypes.NewJSONType(nilToEmptyMap(recoveryHints)),
			"last_error":     message,
			"last_error_at":  now,
		})
	})
}

func (s *GormManagedResourceStore) ClaimOperation(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID uuid.UUID, operation string, leaseUntil time.Time, allowedStatuses []string) (uuid.UUID, error) {
	action := operation
	if operation == "poll" || operation == "recover" {
		action = "read"
	}
	if err := s.authorize(auth, action); err != nil {
		return uuid.Nil, err
	}
	var operationID uuid.UUID
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.FindManagedResourceInTransaction(tx, managedResourceID)
		if err != nil {
			return err
		}
		if err := validateAuthScope(auth, resource.OrganizationID, resource.CanvasID, resource.IntegrationID); err != nil {
			return err
		}
		operationID, err = models.ClaimManagedResourceOperation(tx, managedResourceID, operation, leaseUntil, allowedStatuses)
		return err
	})
	return operationID, err
}

func (s *GormManagedResourceStore) RefreshOperationLease(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, leaseUntil time.Time) error {
	if err := s.authorize(auth, "update"); err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Model(&models.TerraformManagedResource{}).
		Where("managed_resource_id = ?", managedResourceID).
		Where("organization_id = ? AND canvas_id = ? AND integration_id = ?", auth.OrganizationID, auth.CanvasID, auth.IntegrationID).
		Where("current_operation_id = ?", operationID).
		Updates(map[string]any{"operation_expires_at": leaseUntil, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return models.ErrManagedResourceOperationInProgress
	}
	return nil
}

func (s *GormManagedResourceStore) MarkOperationFailed(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, status string, message string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.LockManagedResource(tx, managedResourceID)
		if err != nil {
			return err
		}
		if err := s.authorizeResource(auth, resource, "update"); err != nil {
			return err
		}
		if resource.CurrentOperationID == nil || *resource.CurrentOperationID != operationID {
			return models.ErrManagedResourceOperationInProgress
		}
		now := time.Now()
		updates := map[string]any{
			"health":        models.ManagedResourceHealthDegraded,
			"last_error":    message,
			"last_error_at": now,
		}
		if status != "" {
			updates["status"] = status
		}
		return models.CompleteManagedResourceOperation(tx, managedResourceID, operationID, updates)
	})
}

func (s *GormManagedResourceStore) LoadForOperation(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID) (*LoadedManagedResource, error) {
	loaded, err := s.Load(ctx, auth, managedResourceID)
	if err != nil {
		return nil, err
	}
	if loaded.Resource.CurrentOperationID == nil || *loaded.Resource.CurrentOperationID != operationID {
		return nil, models.ErrManagedResourceOperationInProgress
	}
	return loaded, nil
}

func (s *GormManagedResourceStore) SaveState(ctx context.Context, auth ManagedResourceAuthContext, input SaveManagedResourceStateInput) error {
	return s.saveManagedResourceState(ctx, auth, input, true)
}

func (s *GormManagedResourceStore) SaveRefreshedState(ctx context.Context, auth ManagedResourceAuthContext, input SaveManagedResourceStateInput) error {
	return s.saveManagedResourceState(ctx, auth, input, false)
}

func (s *GormManagedResourceStore) saveManagedResourceState(ctx context.Context, auth ManagedResourceAuthContext, input SaveManagedResourceStateInput, updateConfig bool) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.LockManagedResource(tx, input.ManagedResourceID)
		if err != nil {
			return err
		}
		if err := s.authorizeResource(auth, resource, "update"); err != nil {
			return err
		}
		if resource.CurrentOperationID == nil || *resource.CurrentOperationID != input.OperationID {
			return models.ErrManagedResourceOperationInProgress
		}
		state, err := models.FindManagedResourceState(tx, input.ManagedResourceID)
		if err != nil {
			return err
		}
		stateNonce, err := randomNonce()
		if err != nil {
			return err
		}
		stateCiphertext, err := s.encryptors.Encrypt(ctx, input.StatePayload, managedResourceStateAD(*resource, stateNonce, input.StateFormat), TerraformStateEncryptionV2)
		if err != nil {
			return err
		}
		updates := map[string]any{
			"state_ciphertext": stateCiphertext,
			"state_nonce":      stateNonce,
			"schema_hash":      input.SchemaHash,
			"state_format":     input.StateFormat,
			"lock_version":     gorm.Expr("lock_version + 1"),
			"updated_at":       time.Now(),
		}
		if updateConfig {
			configNonce, err := randomNonce()
			if err != nil {
				return err
			}
			configCiphertext, err := s.encryptors.Encrypt(ctx, input.ConfigPayload, managedResourceConfigAD(*resource, configNonce, input.StateFormat), TerraformStateEncryptionV2)
			if err != nil {
				return err
			}
			updates["last_config_ciphertext"] = configCiphertext
			updates["last_config_nonce"] = configNonce
		}
		result := tx.Model(state).Where("lock_version = ?", input.ExpectedLockVersion).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrLockMismatch
		}
		if updateConfig && input.SanitizedOutputs != nil {
			eventType := input.EventType
			if eventType == "" {
				eventType = models.ManagedResourceEventUpdated
			}
			if _, err := models.CreateManagedResourceEvent(tx, input.ManagedResourceID, eventType, input.SanitizedOutputs, input.HashInput, input.OutputsHash, input.EventMetadata); err != nil {
				return err
			}
		}
		if input.KeepOperationLease {
			return nil
		}
		return models.CompleteManagedResourceOperation(tx, input.ManagedResourceID, input.OperationID, map[string]any{
			"status":            models.ManagedResourceStatusReady,
			"health":            models.ManagedResourceHealthHealthy,
			"last_refreshed_at": time.Now(),
		})
	})
}

func (s *GormManagedResourceStore) RecordMissing(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, threshold int, metadata map[string]any) (string, error) {
	var eventType string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.LockManagedResource(tx, managedResourceID)
		if err != nil {
			return err
		}
		if err := s.authorizeResource(auth, resource, "read"); err != nil {
			return err
		}
		eventType, err = models.RecordManagedResourceMissing(tx, managedResourceID, operationID, threshold)
		if err != nil {
			return err
		}
		_, err = models.CreateManagedResourceEvent(tx, managedResourceID, eventType, map[string]any{}, map[string]any{}, nil, metadata)
		return err
	})
	return eventType, err
}

func (s *GormManagedResourceStore) Delete(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID) error {
	return s.deleteLocal(ctx, auth, managedResourceID, operationID, "delete")
}

func (s *GormManagedResourceStore) ForceForget(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID) error {
	return s.deleteLocal(ctx, auth, managedResourceID, operationID, "force_forget")
}

func (s *GormManagedResourceStore) deleteLocal(ctx context.Context, auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, action string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resource, err := models.LockManagedResource(tx, managedResourceID)
		if err != nil {
			return err
		}
		if err := s.authorizeResource(auth, resource, action); err != nil {
			return err
		}
		if resource.CurrentOperationID != nil && *resource.CurrentOperationID != operationID {
			return models.ErrManagedResourceOperationInProgress
		}
		state, err := models.FindManagedResourceState(tx, managedResourceID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if err := state.ZeroAndDelete(tx); err != nil {
				return err
			}
		}
		eventType := models.ManagedResourceEventDeleted
		if action == "force_forget" {
			eventType = models.ManagedResourceEventForgotten
		}
		if _, err := models.CreateManagedResourceEvent(tx, managedResourceID, eventType, map[string]any{}, map[string]any{}, nil, map[string]any{}); err != nil {
			return err
		}
		now := time.Now()
		return models.CompleteManagedResourceOperation(tx, managedResourceID, operationID, map[string]any{
			"status":     models.ManagedResourceStatusDeleted,
			"deleted_at": now,
		})
	})
}

func (s *GormManagedResourceStore) CountActiveManagedResourcesForIntegration(id uuid.UUID) (int64, error) {
	return models.CountActiveManagedResourcesForIntegration(id)
}

func (s *GormManagedResourceStore) CountActiveManagedResourcesForCanvas(id uuid.UUID) (int64, error) {
	return models.CountActiveManagedResourcesForCanvas(id)
}

func (s *GormManagedResourceStore) CountActiveManagedResourcesForOrganization(id uuid.UUID) (int64, error) {
	return models.CountActiveManagedResourcesForOrganization(id)
}

func (s *GormManagedResourceStore) CountRetainedManagedResourceRowsForIntegration(id uuid.UUID) (int64, error) {
	return models.CountRetainedManagedResourceRowsForIntegration(id)
}

func (s *GormManagedResourceStore) CountRetainedManagedResourceRowsForCanvas(id uuid.UUID) (int64, error) {
	return models.CountRetainedManagedResourceRowsForCanvas(id)
}

func (s *GormManagedResourceStore) CountRetainedManagedResourceRowsForOrganization(id uuid.UUID) (int64, error) {
	return models.CountRetainedManagedResourceRowsForOrganization(id)
}

func (s *GormManagedResourceStore) DeleteExpiredDeletedManagedResources(referenceTime time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		return 0, fmt.Errorf("batch size must be positive")
	}
	return models.DeleteExpiredDeletedManagedResources(referenceTime, batchSize)
}
