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

func Test_AddSSHKey_Setup(t *testing.T) {
	component := &AddSSHKey{}

	t.Run("missing_name", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"data": "ssh-rsa AAAA..."},
		})
		require.ErrorContains(t, err, "name is required")
	})

	t.Run("missing_data", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"name": "my-key"},
		})
		require.ErrorContains(t, err, "data is required")
	})

	t.Run("data_too_large", func(t *testing.T) {
		largeData := "ssh-rsa " + strings.Repeat("A", 16384)
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"name": "my-key", "data": largeData},
		})
		require.ErrorContains(t, err, "16KB")
	})

	t.Run("invalid_prefix", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"name": "my-key", "data": "invalid-key-type AAAA..."},
		})
		require.ErrorContains(t, err, "recognized SSH key prefix")
	})

	t.Run("valid", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"name": "my-key", "data": "ssh-rsa AAAA..."},
		})
		require.NoError(t, err)
	})
}

func Test_AddSSHKey_Execute(t *testing.T) {
	component := &AddSSHKey{}

	t.Run("success", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"key":{"name":"my-key","fingerprint":"ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89","type":"RSA","size":4096}}`,
				)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"name": "my-key", "data": "ssh-rsa AAAA..."},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, AddSSHKeyPayloadType, executionState.Type)
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
			Configuration:  map[string]any{"name": "my-key", "data": "ssh-rsa AAAA..."},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.ErrorContains(t, err, "add ssh key")
	})
}
