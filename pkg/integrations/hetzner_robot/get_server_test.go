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

func Test_GetServer_Setup(t *testing.T) {
	component := &GetServer{}

	t.Run("missing server returns error", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{},
		})
		require.ErrorContains(t, err, "server is required")
	})

	t.Run("valid config passes", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "12345"},
		})
		require.NoError(t, err)
	})
}

func Test_GetServer_Execute(t *testing.T) {
	component := &GetServer{}

	t.Run("successful fetch emits server details", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"server":{"server_number":"12345","server_name":"my-server","product":"EX42","dc":"fsn1-dc14","status":"ready","cancelled":false,"ip":["1.2.3.4"]}}`)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "12345"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, GetServerPayloadType, executionState.Type)
	})

	t.Run("invalid server number returns error", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "../traversal"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.ErrorContains(t, err, "invalid server number")
	})
}
