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

func Test_ResetServer_Setup(t *testing.T) {
	component := &ResetServer{}

	t.Run("missing server returns error", func(t *testing.T) {
		err := component.Setup(core.SetupContext{Configuration: map[string]any{"resetType": "sw"}})
		require.ErrorContains(t, err, "server is required")
	})

	t.Run("missing resetType returns error", func(t *testing.T) {
		err := component.Setup(core.SetupContext{Configuration: map[string]any{"server": "12345"}})
		require.ErrorContains(t, err, "resetType is required")
	})

	t.Run("invalid resetType returns error", func(t *testing.T) {
		err := component.Setup(core.SetupContext{Configuration: map[string]any{"server": "12345", "resetType": "invalid"}})
		require.ErrorContains(t, err, "invalid resetType")
	})

	t.Run("valid config passes", func(t *testing.T) {
		err := component.Setup(core.SetupContext{Configuration: map[string]any{"server": "12345", "resetType": "sw"}})
		require.NoError(t, err)
	})
}

func Test_ResetServer_Execute(t *testing.T) {
	component := &ResetServer{}

	t.Run("successful reset emits payload", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"reset":{"type":"sw"}}`)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "12345", "resetType": "sw"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, ResetServerPayloadType, executionState.Type)
	})

	t.Run("invalid resetType in execute returns error", func(t *testing.T) {
		err := component.Execute(core.ExecutionContext{
			Configuration: map[string]any{"server": "12345", "resetType": "bogus"},
			HTTP:          &contexts.HTTPContext{},
			Integration:   &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}},
		})

		require.ErrorContains(t, err, "invalid resetType")
	})

	t.Run("new-flow credentials work", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"reset":{"type":"sw"}}`)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"username": {Name: "username", Value: []byte("user")},
				"password": {Name: "password", Value: []byte("pass")},
			},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "12345", "resetType": "sw"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, ResetServerPayloadType, executionState.Type)
	})

	t.Run("API error returns error", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"SERVER_ERROR","message":"internal error"}}`)),
			}},
		}
		integrationCtx := &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}}

		err := component.Execute(core.ExecutionContext{
			Configuration: map[string]any{"server": "12345", "resetType": "hw"},
			HTTP:          httpContext,
			Integration:   integrationCtx,
		})

		require.ErrorContains(t, err, "reset server")
	})
}
