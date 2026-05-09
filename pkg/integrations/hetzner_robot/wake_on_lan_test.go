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

func Test_WakeOnLAN_Setup(t *testing.T) {
	component := &WakeOnLAN{}

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

func Test_WakeOnLAN_Execute(t *testing.T) {
	component := &WakeOnLAN{}

	t.Run("successful POST emits on default channel", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"wol":{"server_ip":"1.2.3.4","server_ipv6_net":"2a01:4f8::/64","server_number":"12345"}}`)),
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
		assert.Equal(t, WakeOnLANPayloadType, executionState.Type)
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
