package terraform

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const terraformTelemetryName = "github.com/superplanehq/superplane/pkg/integrations/terraform"

var terraformTelemetry = newTerraformTelemetry(
	otel.Meter(terraformTelemetryName),
	otel.Tracer(terraformTelemetryName),
)

type TerraformTelemetry struct {
	tracer                    trace.Tracer
	providerExecutionTotal    metric.Int64Counter
	providerCacheHitTotal     metric.Int64Counter
	providerCacheMissTotal    metric.Int64Counter
	verificationFailureTotal  metric.Int64Counter
	stateLockConflictTotal    metric.Int64Counter
	providerExecutionDuration metric.Float64Histogram
	concurrentExecutions      metric.Int64UpDownCounter
	queueDepth                metric.Int64UpDownCounter
}

func newTerraformTelemetry(meter metric.Meter, tracer trace.Tracer) *TerraformTelemetry {
	executionTotal, _ := meter.Int64Counter("terraform.provider_execution_total")
	cacheHitTotal, _ := meter.Int64Counter("terraform.provider_cache_hit_total")
	cacheMissTotal, _ := meter.Int64Counter("terraform.provider_cache_miss_total")
	verificationFailureTotal, _ := meter.Int64Counter("terraform.provider_binary_verification_failure_total")
	stateLockConflictTotal, _ := meter.Int64Counter("terraform.provider_state_lock_conflict_total")
	executionDuration, _ := meter.Float64Histogram("terraform.provider_execution_duration_seconds")
	concurrentExecutions, _ := meter.Int64UpDownCounter("terraform.provider_concurrent_executions")
	queueDepth, _ := meter.Int64UpDownCounter("terraform.provider_queue_depth")
	return &TerraformTelemetry{
		tracer:                    tracer,
		providerExecutionTotal:    executionTotal,
		providerCacheHitTotal:     cacheHitTotal,
		providerCacheMissTotal:    cacheMissTotal,
		verificationFailureTotal:  verificationFailureTotal,
		stateLockConflictTotal:    stateLockConflictTotal,
		providerExecutionDuration: executionDuration,
		concurrentExecutions:      concurrentExecutions,
		queueDepth:                queueDepth,
	}
}

func (t *TerraformTelemetry) StartProviderExecution(ctx context.Context, action *GeneratedAction) (context.Context, func(status string, err error)) {
	if t == nil {
		return ctx, func(string, error) {}
	}
	attrs := telemetryActionAttrs(action)
	t.concurrentExecutions.Add(ctx, 1, metric.WithAttributes(attrs...))
	start := time.Now()
	ctx, span := t.startSpan(ctx, "terraform.provider.execution", attrs...)
	return ctx, func(status string, err error) {
		finalAttrs := make([]attribute.KeyValue, len(attrs), len(attrs)+1)
		copy(finalAttrs, attrs)
		finalAttrs = append(finalAttrs, attribute.String("status", status))
		t.providerExecutionTotal.Add(ctx, 1, metric.WithAttributes(finalAttrs...))
		t.providerExecutionDuration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(finalAttrs...))
		t.concurrentExecutions.Add(ctx, -1, metric.WithAttributes(finalAttrs...))
		endSpan(span, status, err)
	}
}

func (t *TerraformTelemetry) StartProviderRPC(ctx context.Context, action *GeneratedAction, rpcName string) (context.Context, func(error)) {
	base := telemetryActionAttrs(action)
	attrs := make([]attribute.KeyValue, len(base), len(base)+1)
	copy(attrs, base)
	attrs = append(attrs, attribute.String("rpc", rpcName))
	return t.StartSpan(ctx, "terraform.provider.rpc", attrs...)
}

func (t *TerraformTelemetry) StartStateOperation(ctx context.Context, action *GeneratedAction, operation string) (context.Context, func(error)) {
	attrs := append(telemetryActionAttrs(action), attribute.String("state_operation", operation))
	return t.StartSpan(ctx, "terraform.provider.state", attrs...)
}

func (t *TerraformTelemetry) RecordStateLockConflict(ctx context.Context, action *GeneratedAction) {
	if t == nil {
		return
	}
	t.stateLockConflictTotal.Add(ctx, 1, metric.WithAttributes(telemetryActionAttrs(action)...))
}

func (t *TerraformTelemetry) StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func(error)) {
	if t == nil {
		return ctx, func(error) {}
	}
	ctx, span := t.startSpan(ctx, name, attrs...)
	return ctx, func(err error) {
		status := "success"
		if err != nil {
			status = "error"
		}
		endSpan(span, status, err)
	}
}

func (t *TerraformTelemetry) StartPluginLaunch(ctx context.Context, providerName, providerSource, providerVersion string, protocolMajor int) (context.Context, func(error)) {
	attrs := append(
		telemetryProviderAttrs(providerName, providerSource, providerVersion),
		attribute.Int("protocol_major", protocolMajor),
	)
	return t.StartSpan(ctx, "terraform.provider.plugin_launch", attrs...)
}

func (t *TerraformTelemetry) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if t == nil || t.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return t.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func (t *TerraformTelemetry) RecordCacheResult(ctx context.Context, providerName, providerSource, providerVersion string, hit bool) {
	if t == nil {
		return
	}
	attrs := telemetryProviderAttrs(providerName, providerSource, providerVersion)
	if hit {
		t.providerCacheHitTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
		return
	}
	t.providerCacheMissTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

func (t *TerraformTelemetry) RecordVerificationFailure(ctx context.Context, providerName, providerSource, providerVersion string) {
	if t == nil {
		return
	}
	t.verificationFailureTotal.Add(ctx, 1, metric.WithAttributes(telemetryProviderAttrs(providerName, providerSource, providerVersion)...))
}

func (t *TerraformTelemetry) AddQueueDepth(ctx context.Context, providerName, providerSource, providerVersion string, delta int64) {
	if t == nil {
		return
	}
	t.queueDepth.Add(ctx, delta, metric.WithAttributes(telemetryProviderAttrs(providerName, providerSource, providerVersion)...))
}

func endSpan(span trace.Span, status string, err error) {
	if span == nil {
		return
	}
	span.SetAttributes(attribute.String("status", status))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "error")
	}
	span.End()
}

func telemetryActionAttrs(action *GeneratedAction) []attribute.KeyValue {
	if action == nil {
		return nil
	}
	return append(
		telemetryProviderAttrs(action.providerName, action.providerSource, action.providerVersion),
		attribute.String("capability", action.capabilityName),
		attribute.String("operation", action.op),
	)
}

func telemetryProviderAttrs(providerName, providerSource, providerVersion string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("provider", providerName),
		attribute.String("source", providerSource),
		attribute.String("version", providerVersion),
	}
}
