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

func Test_UpdateFirewallRule_Setup(t *testing.T) {
	component := &UpdateFirewallRule{}

	t.Run("valid", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{
				"server": "321",
				"name":   "Allow SSH",
				"action": "accept",
			},
		})
		require.NoError(t, err)
	})

	t.Run("missing_server", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"name": "Allow SSH", "action": "accept"},
		})
		require.ErrorContains(t, err, "server is required")
	})

	t.Run("missing_name", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "321", "action": "accept"},
		})
		require.ErrorContains(t, err, "name is required")
	})

	t.Run("missing_action", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "321", "name": "Allow SSH"},
		})
		require.ErrorContains(t, err, "action is required")
	})
}

func Test_UpdateFirewallRule_Execute(t *testing.T) {
	component := &UpdateFirewallRule{}

	t.Run("success", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"firewall":{"server_number":"321","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow SSH","ip_version":"ipv4","protocol":"tcp","dst_port":"22","action":"accept"}],"output":[]}}}`,
					)),
				},
				{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"firewall":{"server_number":"321","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow SSH","ip_version":"ipv4","protocol":"tcp","dst_port":"22","src_ip":"10.0.0.0/8","action":"accept"}],"output":[]}}}`,
					)),
				},
			},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration: map[string]any{
				"server":    "321",
				"name":      "Allow SSH",
				"ipVersion": "ipv4",
				"protocol":  "tcp",
				"dstPort":   "22",
				"srcIp":     "10.0.0.0/8",
				"action":    "accept",
			},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, UpdateFirewallRulePayloadType, executionState.Type)
		require.Len(t, executionState.Payloads, 1)
		wrapped := executionState.Payloads[0].(map[string]any)
		payload := wrapped["data"].(map[string]any)
		assert.Equal(t, "321", payload["serverNumber"])
		assert.Equal(t, 1, payload["ruleCount"])
		rule := payload["rule"].(map[string]any)
		assert.Equal(t, "Allow SSH", rule["name"])
		assert.Equal(t, "10.0.0.0/8", rule["src_ip"])
	})

	t.Run("not_found", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"firewall":{"server_number":"321","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow SSH","ip_version":"ipv4","protocol":"tcp","dst_port":"22","action":"accept"}],"output":[]}}}`,
					)),
				},
			},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration: map[string]any{
				"server": "321",
				"name":   "Allow HTTPS",
				"action": "accept",
			},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.ErrorContains(t, err, "update firewall rule")
	})
}
