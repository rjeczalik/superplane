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

func Test_EnableRescue_Setup(t *testing.T) {
	component := &EnableRescue{}

	t.Run("missing_server", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"os": "linux"},
		})
		require.ErrorContains(t, err, "server is required")
	})

	t.Run("missing_os", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "321"},
		})
		require.ErrorContains(t, err, "os is required")
	})

	t.Run("valid", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "321", "os": "linux"},
		})
		require.NoError(t, err)
	})
}

func Test_EnableRescue_Execute(t *testing.T) {
	component := &EnableRescue{}

	t.Run("success", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"rescue":{"os":"linux","arch":"64","active":true,"password":"secret123","authorized_key":["ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89"]}}`,
				)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321", "os": "linux"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, EnableRescuePayloadType, executionState.Type)
		secret, ok := integrationCtx.CurrentSecrets["rescue-password-321"]
		require.True(t, ok, "rescue-password-321 secret should be set")
		assert.Equal(t, []byte("secret123"), secret.Value)
	})

	t.Run("api_error", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"SERVER_ERROR","message":"internal error"}}`)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321", "os": "linux"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.ErrorContains(t, err, "enable rescue")
	})
}
