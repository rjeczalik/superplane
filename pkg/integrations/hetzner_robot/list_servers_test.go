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

func Test_ListServers_Setup(t *testing.T) {
	t.Run("succeeds with no configuration", func(t *testing.T) {
		err := (&ListServers{}).Setup(core.SetupContext{
			Configuration: map[string]any{},
		})
		require.NoError(t, err)
	})
}

func Test_ListServers_Execute(t *testing.T) {
	t.Run("success emits server list", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`[{"server":{"server_number":"12345","server_name":"my-server","product":"EX42","dc":"fsn1-dc14","status":"ready","cancelled":false,"ip":["1.2.3.4"]}}]`,
			)),
		}
		httpCtx := &contexts.HTTPContext{Responses: []*http.Response{response}}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"username": {Name: "username", Value: []byte("u")},
				"password": {Name: "password", Value: []byte("p")},
			},
		}

		err := (&ListServers{}).Execute(core.ExecutionContext{
			HTTP:           httpCtx,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, ListServersPayloadType, executionState.Type)
		require.Len(t, executionState.Payloads, 1)

		outer := executionState.Payloads[0].(map[string]any)
		payload := outer["data"].(map[string]any)
		assert.Equal(t, 1, payload["serverCount"])
		servers := payload["servers"].([]map[string]any)
		require.Len(t, servers, 1)
		assert.Equal(t, "12345", servers[0]["serverNumber"])
		assert.Equal(t, "my-server", servers[0]["name"])
		assert.Equal(t, "EX42", servers[0]["product"])
		assert.Equal(t, "fsn1-dc14", servers[0]["datacenter"])
		assert.Equal(t, "ready", servers[0]["status"])
		assert.Equal(t, false, servers[0]["cancelled"])
		assert.Equal(t, "1.2.3.4", servers[0]["ipv4"])
	})

	t.Run("api error returns error", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"UNAUTHORIZED","message":"invalid credentials"}}`)),
		}
		httpCtx := &contexts.HTTPContext{Responses: []*http.Response{response}}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"username": {Name: "username", Value: []byte("u")},
				"password": {Name: "password", Value: []byte("p")},
			},
		}

		err := (&ListServers{}).Execute(core.ExecutionContext{
			HTTP:           httpCtx,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "list servers")
	})

	t.Run("empty list emits zero count", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`[]`)),
		}
		httpCtx := &contexts.HTTPContext{Responses: []*http.Response{response}}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"username": {Name: "username", Value: []byte("u")},
				"password": {Name: "password", Value: []byte("p")},
			},
		}

		err := (&ListServers{}).Execute(core.ExecutionContext{
			HTTP:           httpCtx,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, ListServersPayloadType, executionState.Type)
		outer := executionState.Payloads[0].(map[string]any)
		payload := outer["data"].(map[string]any)
		assert.Equal(t, 0, payload["serverCount"])
	})
}
