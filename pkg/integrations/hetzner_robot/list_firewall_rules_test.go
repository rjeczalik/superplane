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

func Test_ListFirewallRules_Setup(t *testing.T) {
	component := &ListFirewallRules{}

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

func Test_ListFirewallRules_Execute(t *testing.T) {
	component := &ListFirewallRules{}

	t.Run("success", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"firewall":{"server_number":"321","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow SSH","ip_version":"ipv4","protocol":"tcp","dst_port":"22","src_ip":"0.0.0.0/0","action":"accept"}],"output":[]}}}`,
				)),
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
		assert.Equal(t, ListFirewallRulesPayloadType, executionState.Type)
		require.Len(t, executionState.Payloads, 1)
		wrapped := executionState.Payloads[0].(map[string]any)
		payload := wrapped["data"].(map[string]any)
		assert.Equal(t, "321", payload["serverNumber"])
		assert.Equal(t, "active", payload["status"])
		assert.Equal(t, true, payload["whitelistHos"])
		assert.Equal(t, 1, payload["ruleCount"])
		rules := payload["rules"].([]any)
		require.Len(t, rules, 1)
		rule := rules[0].(map[string]any)
		assert.Equal(t, "Allow SSH", rule["name"])
		assert.Equal(t, "ipv4", rule["ip_version"])
		assert.Equal(t, "tcp", rule["protocol"])
		assert.Equal(t, "22", rule["dst_port"])
		assert.Equal(t, "0.0.0.0/0", rule["src_ip"])
		assert.Equal(t, "accept", rule["action"])
	})
}
