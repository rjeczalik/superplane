package terraform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/gorm"
)

const defaultManagedResourceTimeout = 30 * time.Minute
const defaultManagedResourceMissingThreshold = 2

type ResourceRunner struct {
	store          ManagedResourceStore
	runtimeFactory ConfiguredRuntimeFactory
	timeout        time.Duration
}

func NewResourceRunner(store ManagedResourceStore, runtimeFactory ConfiguredRuntimeFactory, timeout time.Duration) *ResourceRunner {
	return &ResourceRunner{store: store, runtimeFactory: runtimeFactory, timeout: timeout}
}

func (r *ResourceRunner) Create(execCtx core.ExecutionContext, action *GeneratedAction) error {
	if r.store == nil {
		return fmt.Errorf("managed resource store is required")
	}
	if r.runtimeFactory == nil {
		return fmt.Errorf("terraform runtime factory is required")
	}

	orgID, err := uuid.Parse(execCtx.OrganizationID)
	if err != nil {
		return err
	}
	canvasID, err := uuid.Parse(execCtx.WorkflowID)
	if err != nil {
		return err
	}
	integrationID := execCtx.Integration.ID()
	auth := ManagedResourceSystemAuth(orgID, canvasID, integrationID, "workflow_execution")

	inputs, err := mapConfig(execCtx.Configuration)
	if err != nil {
		return err
	}
	inputs, err = resolveInputs(execCtx, inputs)
	if err != nil {
		return err
	}
	idempotencyKey, onExisting, err := createOptions(inputs)
	if err != nil {
		return err
	}
	delete(inputs, "idempotency_key")
	delete(inputs, "on_existing")
	configValue, err := dynamicValue(inputs)
	if err != nil {
		return err
	}
	providerConfig, err := loadExecutionProviderConfig(execCtx.Integration)
	if err != nil {
		return err
	}
	providerConfigValue, err := dynamicValue(providerConfig)
	if err != nil {
		return err
	}

	timeout := r.timeout
	if timeout == 0 {
		timeout = defaultManagedResourceTimeout
	}
	callCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	operationID := uuid.New()
	managedResourceID := uuid.New()
	if idempotencyKey != nil {
		existing, err := r.store.FindExistingForIdempotency(callCtx, auth, action.terraformType(), *idempotencyKey)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if existing != nil {
			if existing.Status != models.ManagedResourceStatusReady || onExisting != "return_existing" {
				return ErrManagedResourceConflict
			}
			payload := map[string]any{"managed_resource_id": existing.ManagedResourceID.String(), "operation_performed": "existing"}
			loaded, err := r.store.Load(callCtx, auth, existing.ManagedResourceID)
			if err == nil {
				statePayload, err := payloadFromProviderState(runtime.ProviderState{Envelope: loaded.StatePayload})
				if err != nil {
					return err
				}
				sanitized, _, err := SanitizeTerraformOutputs(execCtx.WorkflowID, execCtx.NodeID, existing.ManagedResourceID.String(), "existing", execCtx.Integration, statePayload, action.sensitiveAttrs)
				if err != nil {
					return err
				}
				payload = sanitized
				payload["managed_resource_id"] = existing.ManagedResourceID.String()
				payload["operation_performed"] = "existing"
			}
			return emitTerraformPayload(execCtx, action, payload)
		}
	}
	resource, err := r.store.BeginCreate(callCtx, auth, BeginManagedResourceCreateInput{
		ManagedResourceID:    managedResourceID,
		OperationID:          operationID,
		OrganizationID:       orgID,
		CanvasID:             canvasID,
		IntegrationID:        integrationID,
		CreatedByNodeID:      execCtx.NodeID,
		CreatedByExecutionID: &execCtx.ID,
		ProviderName:         action.providerName,
		ProviderSource:       action.providerSource,
		ProviderVersion:      action.providerVersion,
		ResourceType:         action.terraformType(),
		IdempotencyKey:       idempotencyKey,
		OperationLeaseUntil:  time.Now().Add(timeout + time.Minute),
	})
	if err != nil {
		return err
	}

	provider, err := r.runtimeFactory.RuntimeForProvider(callCtx, config.TerraformProviderIntegration{
		Name:    action.providerName,
		Source:  action.providerSource,
		Version: action.providerVersion,
	})
	if err != nil {
		_ = r.store.MarkCreateProviderFailed(context.Background(), auth, resource.ManagedResourceID, operationID, redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig))
		return err
	}
	defer provider.Close()

	if err := provider.Configure(callCtx, &runtime.ConfigureRequest{Config: providerConfigValue}); err != nil {
		message := redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig)
		_ = r.store.MarkCreateProviderFailed(context.Background(), auth, resource.ManagedResourceID, operationID, message)
		return fmt.Errorf("%s", message)
	}
	if err := provider.ValidateResourceConfig(callCtx, &runtime.ValidateResourceConfigRequest{TypeName: action.terraformType(), Config: configValue}); err != nil {
		message := redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig)
		_ = r.store.MarkCreateProviderFailed(context.Background(), auth, resource.ManagedResourceID, operationID, message)
		return fmt.Errorf("%s", message)
	}
	result, err := provider.CreateResource(callCtx, &runtime.CreateResourceRequest{TypeName: action.terraformType(), Config: configValue, SchemaHash: action.schemaHash})
	if err != nil {
		message := redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig)
		_ = r.store.MarkCreateOrphanRisk(context.Background(), auth, resource.ManagedResourceID, operationID, message, map[string]any{})
		return fmt.Errorf("%s", message)
	}

	payload, err := payloadFromProviderState(result.NewState)
	if err != nil {
		_ = r.store.MarkCreateOrphanRisk(context.Background(), auth, resource.ManagedResourceID, operationID, redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig), map[string]any{})
		return err
	}
	sanitized, hashInput, err := SanitizeTerraformOutputs(execCtx.WorkflowID, execCtx.NodeID, resource.ManagedResourceID.String(), operationID.String(), execCtx.Integration, payload, action.sensitiveAttrs)
	if err != nil {
		_ = r.store.MarkCreateOrphanRisk(context.Background(), auth, resource.ManagedResourceID, operationID, redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig), map[string]any{})
		return err
	}
	outputsHash, err := hashTerraformOutputs(hashInput)
	if err != nil {
		_ = r.store.MarkCreateOrphanRisk(context.Background(), auth, resource.ManagedResourceID, operationID, redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig), map[string]any{})
		return err
	}
	configPayload, err := json.Marshal(inputs)
	if err != nil {
		_ = r.store.MarkCreateOrphanRisk(context.Background(), auth, resource.ManagedResourceID, operationID, redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig), map[string]any{})
		return err
	}
	if err := r.store.CompleteCreate(context.Background(), auth, CompleteManagedResourceCreateInput{
		ManagedResourceID: resource.ManagedResourceID,
		OperationID:       operationID,
		StatePayload:      result.NewState.Envelope,
		ConfigPayload:     configPayload,
		SchemaHash:        action.schemaHash,
		StateFormat:       TerraformStateFormatRuntime,
		SanitizedOutputs:  sanitized,
		HashInput:         hashInput,
		OutputsHash:       &outputsHash,
		EventMetadata:     map[string]any{"operation": "create"},
	}); err != nil {
		_ = r.store.MarkCreateOrphanRisk(context.Background(), auth, resource.ManagedResourceID, operationID, redactTerraformErrorMessage(err, action.sensitiveAttrs, inputs, providerConfig), map[string]any{})
		return err
	}

	return emitTerraformPayload(execCtx, action, sanitized)
}

func (r *ResourceRunner) Read(execCtx core.ExecutionContext, action *GeneratedAction) error {
	managedResourceID, err := managedResourceIDFromConfig(execCtx.Configuration)
	if err != nil {
		return err
	}
	auth, err := r.systemAuth(execCtx)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(context.Background(), r.effectiveTimeout())
	defer cancel()

	operationID, err := r.store.ClaimOperation(callCtx, auth, managedResourceID, "read", time.Now().Add(r.effectiveTimeout()+time.Minute), []string{
		"ready", "missing", "updating", "deleting",
	})
	if err != nil {
		return err
	}
	loaded, err := r.store.LoadForOperation(callCtx, auth, managedResourceID, operationID)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, "", err)
		return err
	}
	provider, err := r.configuredProvider(callCtx, execCtx, action)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, loaded.Resource.Status, err)
		return err
	}
	defer provider.Close()

	result, err := provider.ReadResourceState(callCtx, &runtime.ReadResourceStateRequest{
		TypeName:   loaded.Resource.ResourceType,
		PriorState: runtime.ProviderState{Envelope: loaded.StatePayload},
		SchemaHash: action.schemaHash,
	})
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, loaded.Resource.Status, err)
		return err
	}
	if result.NotFound {
		eventType, err := r.store.RecordMissing(context.Background(), auth, managedResourceID, operationID, defaultManagedResourceMissingThreshold, map[string]any{"operation": "read"})
		if err != nil {
			r.markOperationFailed(auth, managedResourceID, operationID, loaded.Resource.Status, err)
			return err
		}
		return emitTerraformPayload(execCtx, action, map[string]any{
			"managed_resource_id": managedResourceID.String(),
			"event_type":          eventType,
			"operation_performed": "missing",
		})
	}
	payload, err := payloadFromProviderState(result.NewState)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, loaded.Resource.Status, err)
		return err
	}
	sanitized, hashInput, err := SanitizeTerraformOutputs(execCtx.WorkflowID, execCtx.NodeID, managedResourceID.String(), operationID.String(), execCtx.Integration, payload, action.sensitiveAttrs)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, loaded.Resource.Status, err)
		return err
	}
	if err := r.store.SaveRefreshedState(context.Background(), auth, SaveManagedResourceStateInput{
		ManagedResourceID:   managedResourceID,
		OperationID:         operationID,
		ExpectedLockVersion: loaded.State.LockVersion,
		StatePayload:        result.NewState.Envelope,
		SchemaHash:          action.schemaHash,
		StateFormat:         TerraformStateFormatRuntime,
	}); err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, loaded.Resource.Status, err)
		return err
	}
	_ = hashInput
	return emitTerraformPayload(execCtx, action, sanitized)
}

func (r *ResourceRunner) Update(execCtx core.ExecutionContext, action *GeneratedAction) error {
	managedResourceID, err := managedResourceIDFromConfig(execCtx.Configuration)
	if err != nil {
		return err
	}
	auth, err := r.systemAuth(execCtx)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(context.Background(), r.effectiveTimeout())
	defer cancel()

	operationID, err := r.store.ClaimOperation(callCtx, auth, managedResourceID, "update", time.Now().Add(r.effectiveTimeout()+time.Minute), []string{"ready"})
	if err != nil {
		return err
	}
	loaded, err := r.store.LoadForOperation(callCtx, auth, managedResourceID, operationID)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	inputs, err := mapConfig(execCtx.Configuration)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	inputs, err = resolveInputs(execCtx, inputs)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	delete(inputs, "managed_resource_id")
	strategy, err := replacementStrategy(inputs)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	delete(inputs, "replacement_strategy")
	mergedConfig, err := mergeConfigPatch(loaded.ConfigPayload, inputs)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	configValue, err := dynamicValue(mergedConfig)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	provider, err := r.configuredProvider(callCtx, execCtx, action)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	defer provider.Close()

	refresh, err := provider.ReadResourceState(callCtx, &runtime.ReadResourceStateRequest{
		TypeName:   loaded.Resource.ResourceType,
		PriorState: runtime.ProviderState{Envelope: loaded.StatePayload},
		SchemaHash: action.schemaHash,
	})
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	if refresh.NotFound {
		eventType, err := r.store.RecordMissing(context.Background(), auth, managedResourceID, operationID, defaultManagedResourceMissingThreshold, map[string]any{"operation": "update_refresh"})
		if err != nil {
			r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
			return err
		}
		return emitTerraformPayload(execCtx, action, map[string]any{
			"managed_resource_id": managedResourceID.String(),
			"event_type":          eventType,
			"operation_performed": "missing",
		})
	}

	plan, err := provider.PlanResourceChange(callCtx, &runtime.PlanResourceChangeRequest{
		TypeName:      loaded.Resource.ResourceType,
		PriorState:    refresh.NewState,
		Config:        configValue,
		ProposedState: configValue,
	})
	if err != nil {
		if saveErr := r.store.SaveRefreshedState(context.Background(), auth, SaveManagedResourceStateInput{
			ManagedResourceID:   managedResourceID,
			OperationID:         operationID,
			ExpectedLockVersion: loaded.State.LockVersion,
			StatePayload:        refresh.NewState.Envelope,
			SchemaHash:          action.schemaHash,
			StateFormat:         TerraformStateFormatRuntime,
		}); saveErr != nil {
			r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, saveErr)
		}
		return err
	}
	replacementRequired := len(plan.ReplacementMetadata.RequiresReplace) > 0
	if strategy == "preview_only" {
		if err := r.store.SaveRefreshedState(context.Background(), auth, SaveManagedResourceStateInput{
			ManagedResourceID:   managedResourceID,
			OperationID:         operationID,
			ExpectedLockVersion: loaded.State.LockVersion,
			StatePayload:        refresh.NewState.Envelope,
			SchemaHash:          action.schemaHash,
			StateFormat:         TerraformStateFormatRuntime,
		}); err != nil {
			r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
			return err
		}
		return emitTerraformPayload(execCtx, action, map[string]any{
			"managed_resource_id": managedResourceID.String(),
			"operation_performed": "preview",
			"replacement": map[string]any{
				"required":         replacementRequired,
				"requires_replace": plan.ReplacementMetadata.RequiresReplace,
			},
		})
	}
	if replacementRequired && strategy != "replace" {
		if err := r.store.SaveRefreshedState(context.Background(), auth, SaveManagedResourceStateInput{
			ManagedResourceID:   managedResourceID,
			OperationID:         operationID,
			ExpectedLockVersion: loaded.State.LockVersion,
			StatePayload:        refresh.NewState.Envelope,
			SchemaHash:          action.schemaHash,
			StateFormat:         TerraformStateFormatRuntime,
		}); err != nil {
			r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
			return err
		}
		return fmt.Errorf("terraform update requires replacement; set replacement_strategy=replace or preview_only")
	}
	if err := r.store.SaveRefreshedState(context.Background(), auth, SaveManagedResourceStateInput{
		ManagedResourceID:   managedResourceID,
		OperationID:         operationID,
		ExpectedLockVersion: loaded.State.LockVersion,
		StatePayload:        refresh.NewState.Envelope,
		SchemaHash:          action.schemaHash,
		StateFormat:         TerraformStateFormatRuntime,
		KeepOperationLease:  true,
	}); err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	apply, err := provider.ApplyResourceChange(callCtx, &runtime.ApplyResourceChangeRequest{
		TypeName:     loaded.Resource.ResourceType,
		PriorState:   refresh.NewState,
		PlannedState: plan.PlannedState,
		Config:       configValue,
	})
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	payload, err := payloadFromProviderState(apply.NewState)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	sanitized, hashInput, err := SanitizeTerraformOutputs(execCtx.WorkflowID, execCtx.NodeID, managedResourceID.String(), operationID.String(), execCtx.Integration, payload, action.sensitiveAttrs)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	outputsHash, err := hashTerraformOutputs(hashInput)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	configPayload, err := json.Marshal(mergedConfig)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	eventType := models.ManagedResourceEventUpdated
	if replacementRequired {
		eventType = models.ManagedResourceEventReplaced
	}
	if err := r.store.SaveState(context.Background(), auth, SaveManagedResourceStateInput{
		ManagedResourceID:   managedResourceID,
		OperationID:         operationID,
		ExpectedLockVersion: loaded.State.LockVersion + 1,
		StatePayload:        apply.NewState.Envelope,
		ConfigPayload:       configPayload,
		SchemaHash:          action.schemaHash,
		StateFormat:         TerraformStateFormatRuntime,
		SanitizedOutputs:    sanitized,
		HashInput:           hashInput,
		OutputsHash:         &outputsHash,
		EventMetadata:       map[string]any{"operation": "update"},
		EventType:           eventType,
	}); err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	return emitTerraformPayload(execCtx, action, sanitized)
}

func (r *ResourceRunner) Delete(execCtx core.ExecutionContext, action *GeneratedAction) error {
	managedResourceID, err := managedResourceIDFromConfig(execCtx.Configuration)
	if err != nil {
		return err
	}
	inputs, err := mapConfig(execCtx.Configuration)
	if err != nil {
		return err
	}
	forceForget, _ := inputs["force_forget"].(bool)
	if forceForget {
		if confirmed, _ := inputs["confirm_forget"].(bool); !confirmed {
			return fmt.Errorf("confirm_forget=true is required when force_forget=true")
		}
	} else if confirmed, _ := inputs["confirm_delete"].(bool); !confirmed {
		return fmt.Errorf("confirm_delete=true is required")
	}
	auth, err := r.systemAuth(execCtx)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(context.Background(), r.effectiveTimeout())
	defer cancel()

	operation := "delete"
	if forceForget {
		operation = "force_forget"
	}
	operationID, err := r.store.ClaimOperation(callCtx, auth, managedResourceID, operation, time.Now().Add(r.effectiveTimeout()+time.Minute), []string{"ready", "missing", "creating"})
	if err != nil {
		return err
	}
	if forceForget {
		if err := r.store.ForceForget(context.Background(), auth, managedResourceID, operationID); err != nil {
			r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
			return err
		}
		return emitTerraformPayload(execCtx, action, map[string]any{"managed_resource_id": managedResourceID.String(), "operation_performed": "forgotten"})
	}
	loaded, err := r.store.LoadForOperation(callCtx, auth, managedResourceID, operationID)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	provider, err := r.configuredProvider(callCtx, execCtx, action)
	if err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	defer provider.Close()
	if _, err := provider.DeleteResource(callCtx, &runtime.DeleteResourceRequest{
		TypeName:   loaded.Resource.ResourceType,
		PriorState: runtime.ProviderState{Envelope: loaded.StatePayload},
		SchemaHash: action.schemaHash,
	}); err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	if err := r.store.Delete(context.Background(), auth, managedResourceID, operationID); err != nil {
		r.markOperationFailed(auth, managedResourceID, operationID, models.ManagedResourceStatusReady, err)
		return err
	}
	return emitTerraformPayload(execCtx, action, map[string]any{"managed_resource_id": managedResourceID.String(), "operation_performed": "deleted"})
}

func (r *ResourceRunner) configuredProvider(ctx context.Context, execCtx core.ExecutionContext, action *GeneratedAction) (runtime.ProviderRuntime, error) {
	providerConfig, err := loadExecutionProviderConfig(execCtx.Integration)
	if err != nil {
		return nil, err
	}
	providerConfigValue, err := dynamicValue(providerConfig)
	if err != nil {
		return nil, err
	}
	provider, err := r.runtimeFactory.RuntimeForProvider(ctx, config.TerraformProviderIntegration{
		Name:    action.providerName,
		Source:  action.providerSource,
		Version: action.providerVersion,
	})
	if err != nil {
		return nil, err
	}
	if err := provider.Configure(ctx, &runtime.ConfigureRequest{Config: providerConfigValue}); err != nil {
		_ = provider.Close()
		return nil, err
	}
	return provider, nil
}

func (r *ResourceRunner) systemAuth(execCtx core.ExecutionContext) (ManagedResourceAuthContext, error) {
	orgID, err := uuid.Parse(execCtx.OrganizationID)
	if err != nil {
		return ManagedResourceAuthContext{}, err
	}
	canvasID, err := uuid.Parse(execCtx.WorkflowID)
	if err != nil {
		return ManagedResourceAuthContext{}, err
	}
	return ManagedResourceSystemAuth(orgID, canvasID, execCtx.Integration.ID(), "workflow_execution"), nil
}

func (r *ResourceRunner) effectiveTimeout() time.Duration {
	if r.timeout != 0 {
		return r.timeout
	}
	return defaultManagedResourceTimeout
}

func (r *ResourceRunner) markOperationFailed(auth ManagedResourceAuthContext, managedResourceID, operationID uuid.UUID, status string, err error) {
	if err == nil {
		return
	}
	_ = r.store.MarkOperationFailed(context.Background(), auth, managedResourceID, operationID, status, err.Error())
}

func managedResourceIDFromConfig(configuration any) (uuid.UUID, error) {
	inputs, err := mapConfig(configuration)
	if err != nil {
		return uuid.Nil, err
	}
	raw, ok := inputs["managed_resource_id"].(string)
	if !ok || raw == "" {
		return uuid.Nil, fmt.Errorf("managed_resource_id is required")
	}
	return uuid.Parse(raw)
}

func replacementStrategy(inputs map[string]any) (string, error) {
	raw, ok := inputs["replacement_strategy"]
	if !ok || raw == nil || raw == "" {
		return "fail", nil
	}
	strategy, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("replacement_strategy must be a string")
	}
	switch strategy {
	case "fail", "replace", "preview_only":
		return strategy, nil
	default:
		return "", fmt.Errorf("unsupported replacement_strategy %q", strategy)
	}
}

func createOptions(inputs map[string]any) (*string, string, error) {
	var idempotencyKey *string
	if raw, ok := inputs["idempotency_key"]; ok && raw != nil && raw != "" {
		value, ok := raw.(string)
		if !ok {
			return nil, "", fmt.Errorf("idempotency_key must be a string")
		}
		idempotencyKey = &value
	}

	onExisting := "fail"
	if raw, ok := inputs["on_existing"]; ok && raw != nil && raw != "" {
		value, ok := raw.(string)
		if !ok {
			return nil, "", fmt.Errorf("on_existing must be a string")
		}
		onExisting = value
	}
	switch onExisting {
	case "fail", "return_existing":
		return idempotencyKey, onExisting, nil
	default:
		return nil, "", fmt.Errorf("unsupported on_existing %q", onExisting)
	}
}

func mergeConfigPatch(previousPayload []byte, patch map[string]any) (map[string]any, error) {
	base := map[string]any{}
	if len(previousPayload) > 0 {
		if err := json.Unmarshal(previousPayload, &base); err != nil {
			return nil, fmt.Errorf("decode previous terraform config: %w", err)
		}
	}
	return mergeConfigMap(base, patch), nil
}

func mergeConfigMap(base map[string]any, patch map[string]any) map[string]any {
	out := cloneMap(base)
	for key, value := range patch {
		if value == nil {
			delete(out, key)
			continue
		}
		patchMap, patchIsMap := value.(map[string]any)
		baseMap, baseIsMap := out[key].(map[string]any)
		if patchIsMap && baseIsMap {
			out[key] = mergeConfigMap(baseMap, patchMap)
			continue
		}
		out[key] = cloneAny(value)
	}
	return out
}

func redactTerraformErrorMessage(err error, sensitiveAttrs map[string]struct{}, payloads ...map[string]any) string {
	if err == nil {
		return ""
	}
	values := make([]string, 0)
	for _, payload := range payloads {
		values = append(values, stringLeaves(payload)...)
	}
	redacted := RedactTerraformDiagnostics(err.Error(), sensitiveAttrs, values)
	if msg, ok := redacted.(string); ok {
		return msg
	}
	raw, marshalErr := json.Marshal(redacted)
	if marshalErr != nil {
		return err.Error()
	}
	return string(raw)
}

func stringLeaves(value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		var out []string
		for _, child := range typed {
			out = append(out, stringLeaves(child)...)
		}
		return out
	case []any:
		var out []string
		for _, child := range typed {
			out = append(out, stringLeaves(child)...)
		}
		return out
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}

func hashTerraformOutputs(payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
