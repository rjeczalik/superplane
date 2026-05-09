package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

const defaultDirectRunnerTimeout = 30 * time.Minute

type DirectRuntimeFactory func(ctx context.Context, spec *runtime.ExecutionSpec) (runtime.ProviderRuntime, error)

type DirectRunner struct {
	runtimeFactory DirectRuntimeFactory
	timeout        time.Duration
	saveRetries    int
	audit          *AuditLogger
}

type DirectRunnerOption func(*DirectRunner)

func WithDirectRunnerAudit(audit *AuditLogger) DirectRunnerOption {
	return func(r *DirectRunner) {
		r.audit = audit
	}
}

func NewDirectRunner(runtimeFactory DirectRuntimeFactory, timeout time.Duration, options ...DirectRunnerOption) *DirectRunner {
	runner := &DirectRunner{
		runtimeFactory: runtimeFactory,
		timeout:        timeout,
		saveRetries:    2,
	}
	for _, option := range options {
		option(runner)
	}
	return runner
}

func (r *DirectRunner) Execute(execCtx core.ExecutionContext, action *GeneratedAction) (err error) {
	started := time.Now()
	status := "success"
	telemetryCtx, finishTelemetry := terraformTelemetry.StartProviderExecution(context.Background(), action)
	defer func() {
		r.auditExecution(action, status, time.Since(started))
		finishTelemetry(status, err)
	}()

	if err := requireCapabilityEnabled(execCtx.Integration, action.capabilityName); err != nil {
		status = "capability_disabled"
		return err
	}
	if r.runtimeFactory == nil {
		status = "runtime_factory_missing"
		return fmt.Errorf("terraform direct runtime factory is required")
	}

	inputs, err := mapConfig(execCtx.Configuration)
	if err != nil {
		status = "input_error"
		return err
	}
	inputs, err = resolveInputs(execCtx, inputs)
	if err != nil {
		status = "input_error"
		return err
	}
	configValue, err := dynamicValue(inputs)
	if err != nil {
		status = "input_error"
		return err
	}
	providerConfig, err := loadExecutionProviderConfig(execCtx.Integration)
	if err != nil {
		status = "provider_config_error"
		return err
	}
	providerConfigValue, err := dynamicValue(providerConfig)
	if err != nil {
		status = "provider_config_error"
		return err
	}

	timeout := r.timeout
	if timeout == 0 {
		timeout = defaultDirectRunnerTimeout
	}
	callCtx, cancel := context.WithTimeout(telemetryCtx, timeout)
	defer cancel()

	spec := executionSpec(action)
	provider, err := r.runtimeFactory(callCtx, spec)
	if err != nil {
		status = "runtime_create_error"
		return err
	}
	defer provider.Close()

	r.auditConfigure(action)
	rpcCtx, finishRPC := terraformTelemetry.StartProviderRPC(callCtx, action, "Configure")
	err = provider.Configure(rpcCtx, &runtime.ConfigureRequest{Config: providerConfigValue})
	finishRPC(err)
	if err != nil {
		status = "configure_error"
		return err
	}

	switch action.op {
	case "data":
		err = r.executeRead(callCtx, execCtx, action, provider, configValue)
	case "action":
		err = r.executeAction(callCtx, execCtx, action, provider, configValue)
	default:
		err = fmt.Errorf("unsupported terraform operation %q", action.op)
	}
	if err != nil {
		status = "execution_error"
	}
	return err
}

func (r *DirectRunner) executeRead(ctx context.Context, execCtx core.ExecutionContext, action *GeneratedAction, provider runtime.ProviderRuntime, config runtime.DynamicValue) error {
	typeName := action.terraformType()
	rpcCtx, finishRPC := terraformTelemetry.StartProviderRPC(ctx, action, "ValidateDataSourceConfig")
	err := provider.ValidateDataSourceConfig(rpcCtx, &runtime.ValidateDataSourceConfigRequest{TypeName: typeName, Config: config})
	finishRPC(err)
	if err != nil {
		return err
	}
	rpcCtx, finishRPC = terraformTelemetry.StartProviderRPC(ctx, action, "ReadDataSource")
	resp, err := provider.ReadDataSource(rpcCtx, &runtime.ReadDataSourceRequest{TypeName: typeName, Config: config})
	finishRPC(err)
	if err != nil {
		return err
	}
	payload, err := payloadFromDynamicValue(resp.State)
	if err != nil {
		return err
	}
	if err := filterSensitiveOutputs(execCtx, payload, action.sensitiveAttrs, execCtx.WorkflowID, execCtx.NodeID); err != nil {
		return err
	}
	return emitTerraformPayload(execCtx, action, payload)
}

func (r *DirectRunner) executeAction(ctx context.Context, execCtx core.ExecutionContext, action *GeneratedAction, provider runtime.ProviderRuntime, config runtime.DynamicValue) error {
	typeName := action.terraformType()
	rpcCtx, finishRPC := terraformTelemetry.StartProviderRPC(ctx, action, "ValidateAction")
	err := provider.ValidateAction(rpcCtx, &runtime.ValidateActionRequest{TypeName: typeName, Config: config})
	finishRPC(err)
	if err != nil {
		return err
	}
	if action.hasPlanStep {
		rpcCtx, finishRPC = terraformTelemetry.StartProviderRPC(ctx, action, "PlanAction")
		planned, err := provider.PlanAction(rpcCtx, &runtime.PlanActionRequest{TypeName: typeName, Config: config})
		finishRPC(err)
		if err != nil {
			return err
		}
		if planned.Deferred {
			return fmt.Errorf("terraform action %q was deferred by provider", action.capabilityName)
		}
	}
	rpcCtx, finishRPC = terraformTelemetry.StartProviderRPC(ctx, action, "InvokeAction")
	resp, err := provider.InvokeAction(rpcCtx, &runtime.InvokeActionRequest{TypeName: typeName, Config: config})
	finishRPC(err)
	if err != nil {
		return err
	}
	events := make([]map[string]any, 0, len(resp.Events))
	for _, event := range resp.Events {
		events = append(events, map[string]any{
			"type":    event.Type,
			"message": event.Message,
		})
	}
	payload := map[string]any{"events": events}
	if err := filterSensitiveOutputs(execCtx, payload, action.sensitiveAttrs, execCtx.WorkflowID, execCtx.NodeID); err != nil {
		return err
	}
	return emitTerraformPayload(execCtx, action, payload)
}

func executionSpec(action *GeneratedAction) *runtime.ExecutionSpec {
	op := runtime.Operation(action.op)
	return &runtime.ExecutionSpec{
		CapabilityName:  action.capabilityName,
		CapabilityKind:  op,
		ProviderName:    action.providerName,
		ProviderSource:  action.providerSource,
		ProviderVersion: action.providerVersion,
		ResourceName:    action.resourceName,
		Operation:       op,
		SchemaHash:      action.schemaHash,
		SensitiveAttrs:  action.sensitiveAttrs,
		HasPlanStep:     action.hasPlanStep,
	}
}

func dynamicValue(value any) (runtime.DynamicValue, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return runtime.DynamicValue{}, err
	}
	return runtime.DynamicValue{JSON: raw}, nil
}

func payloadFromDynamicValue(value runtime.DynamicValue) (map[string]any, error) {
	if len(value.JSON) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(value.JSON, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func payloadFromProviderState(state runtime.ProviderState) (map[string]any, error) {
	if len(state.Envelope) == 0 {
		return map[string]any{}, nil
	}
	var envelope runtime.StateEnvelope
	if err := json.Unmarshal(state.Envelope, &envelope); err != nil {
		return nil, fmt.Errorf("decode provider state envelope: %w", err)
	}
	return payloadFromDynamicValue(envelope.Value)
}

func PayloadFromProviderState(state runtime.ProviderState) (map[string]any, error) {
	return payloadFromProviderState(state)
}

func emitTerraformPayload(ctx core.ExecutionContext, action *GeneratedAction, payload map[string]any) error {
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, "terraform."+action.resourceName+"."+action.op, []any{payload})
}

func (r *DirectRunner) auditConfigure(action *GeneratedAction) {
	if r.audit != nil {
		r.audit.LogConfigureRPC(action.providerName, action.providerSource, action.providerVersion, action.capabilityName)
	}
}

func (r *DirectRunner) auditExecution(action *GeneratedAction, status string, duration time.Duration) {
	if r.audit != nil {
		r.audit.LogCapabilityExecution(action.providerName, action.providerSource, action.providerVersion, action.capabilityName, action.op, status, duration)
	}
}

func (r *DirectRunner) auditStateRead(action *GeneratedAction, status string) {
	if r.audit != nil {
		r.audit.LogStateRead(action.providerName, action.providerSource, action.providerVersion, action.capabilityName, status)
	}
}

func (r *DirectRunner) auditStateWrite(action *GeneratedAction, status string) {
	if r.audit != nil {
		r.audit.LogStateWrite(action.providerName, action.providerSource, action.providerVersion, action.capabilityName, status)
	}
}
