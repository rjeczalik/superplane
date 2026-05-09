package hetznerrobot

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/test/support/contexts"
)

func Test_CancelLinuxInstall_Setup(t *testing.T) {
	component := &CancelLinuxInstall{}

	t.Run("missing_server", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{},
		})
		require.ErrorContains(t, err, "server is required")
	})

	t.Run("valid", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "321"},
		})
		require.NoError(t, err)
	})
}

func Test_CancelLinuxInstall_Execute(t *testing.T) {
	component := &CancelLinuxInstall{}

	t.Run("success", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, CancelLinuxInstallPayloadType, executionState.Type)
	})

	t.Run("idempotent_404", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"NOT_FOUND","message":"No active linux config"}}`)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, CancelLinuxInstallPayloadType, executionState.Type)
	})

	t.Run("other_error", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"SERVER_ERROR","message":"Internal error"}}`)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.ErrorContains(t, err, "cancel linux install")
	})
}
