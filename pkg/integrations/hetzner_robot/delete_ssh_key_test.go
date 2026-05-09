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

func Test_DeleteSSHKey_Setup(t *testing.T) {
	component := &DeleteSSHKey{}

	t.Run("missing_fingerprint", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{},
		})
		require.ErrorContains(t, err, "fingerprint is required")
	})

	t.Run("invalid_fingerprint", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"fingerprint": "not-a-valid-fingerprint"},
		})
		require.ErrorContains(t, err, "invalid fingerprint")
	})

	t.Run("valid", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"fingerprint": "ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89"},
		})
		require.NoError(t, err)
	})
}

func Test_DeleteSSHKey_Execute(t *testing.T) {
	component := &DeleteSSHKey{}

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
			Configuration:  map[string]any{"fingerprint": "ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, DeleteSSHKeyPayloadType, executionState.Type)
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
			Configuration:  map[string]any{"fingerprint": "ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.ErrorContains(t, err, "delete ssh key")
	})
}
