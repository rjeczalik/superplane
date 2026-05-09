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

func Test_RenameServer_Setup(t *testing.T) {
	component := &RenameServer{}

	t.Run("missing server returns error", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"name": "new-name"},
		})
		require.ErrorContains(t, err, "server is required")
	})

	t.Run("missing name returns error", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "12345"},
		})
		require.ErrorContains(t, err, "name is required")
	})

	t.Run("valid config passes", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "12345", "name": "new-name"},
		})
		require.NoError(t, err)
	})
}

func Test_RenameServer_Execute(t *testing.T) {
	component := &RenameServer{}

	t.Run("successful rename emits server details", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"server":{"server_number":"12345","server_name":"new-name","product":"EX42","dc":"FSN1-DC14","status":"ready","cancelled":false,"ip":["1.2.3.4"],"subnet":[]}}`)),
			}},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "12345", "name": "new-name"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, RenameServerPayloadType, executionState.Type)
		require.Len(t, executionState.Payloads, 1)

		wrapped := executionState.Payloads[0].(map[string]any)
		payload := wrapped["data"].(map[string]any)
		assert.Equal(t, "12345", payload["serverNumber"])
		assert.Equal(t, "new-name", payload["name"])
		assert.Equal(t, "EX42", payload["product"])
		assert.Equal(t, "FSN1-DC14", payload["datacenter"])
		assert.Equal(t, "ready", payload["status"])
		assert.Equal(t, false, payload["cancelled"])
		assert.Equal(t, "1.2.3.4", payload["ipv4"])
	})

	t.Run("invalid server number returns error", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "../traversal", "name": "new-name"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.ErrorContains(t, err, "invalid server number")
	})
}
