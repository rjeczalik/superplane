package terraform

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTerraformTelemetryMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	telemetry := newTerraformTelemetry(provider.Meter(terraformTelemetryName), otel.Tracer(terraformTelemetryName))
	action := runnerTestAction()

	ctx, finish := telemetry.StartProviderExecution(context.Background(), action)
	finish("success", nil)
	telemetry.RecordCacheResult(ctx, action.providerName, action.providerSource, action.providerVersion, true)
	telemetry.RecordCacheResult(ctx, action.providerName, action.providerSource, action.providerVersion, false)
	telemetry.RecordVerificationFailure(ctx, action.providerName, action.providerSource, action.providerVersion)
	telemetry.RecordStateLockConflict(ctx, action)
	telemetry.AddQueueDepth(ctx, action.providerName, action.providerSource, action.providerVersion, 1)
	telemetry.AddQueueDepth(ctx, action.providerName, action.providerSource, action.providerVersion, -1)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))
	names := metricNames(rm)
	assert.Contains(t, names, "terraform.provider_execution_total")
	assert.Contains(t, names, "terraform.provider_cache_hit_total")
	assert.Contains(t, names, "terraform.provider_cache_miss_total")
	assert.Contains(t, names, "terraform.provider_binary_verification_failure_total")
	assert.Contains(t, names, "terraform.provider_state_lock_conflict_total")
	assert.Contains(t, names, "terraform.provider_execution_duration_seconds")
	assert.Contains(t, names, "terraform.provider_concurrent_executions")
	assert.Contains(t, names, "terraform.provider_queue_depth")
}

func TestTerraformTelemetrySpansUseMetadataOnly(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	telemetry := newTerraformTelemetry(otel.Meter(terraformTelemetryName), provider.Tracer(terraformTelemetryName))
	action := runnerTestAction()

	ctx, finishExecution := telemetry.StartProviderExecution(context.Background(), action)
	_, finishRPC := telemetry.StartProviderRPC(ctx, action, "Configure")
	finishRPC(nil)
	_, finishState := telemetry.StartStateOperation(ctx, action, "load")
	finishState(nil)
	time.Sleep(time.Millisecond)
	finishExecution("success", nil)

	require.Len(t, recorder.Ended(), 3)
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			value := attr.Value.AsString()
			assert.NotContains(t, value, "secret-token")
			assert.NotContains(t, value, "password")
			assert.NotContains(t, value, "terraform.tfstate")
			assert.NotContains(t, value, "provider stderr")
		}
	}
}

func metricNames(rm metricdata.ResourceMetrics) map[string]struct{} {
	names := map[string]struct{}{}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			names[metric.Name] = struct{}{}
		}
	}
	return names
}
