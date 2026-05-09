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

func Test_AddFirewallRule_Setup(t *testing.T) {
	component := &AddFirewallRule{}

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

	t.Run("invalid_cidr", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{
				"server": "321",
				"name":   "Allow SSH",
				"action": "accept",
				"srcIp":  "bad",
			},
		})
		require.ErrorContains(t, err, "invalid CIDR")
	})

	t.Run("invalid_port", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{
				"server":  "321",
				"name":    "Allow SSH",
				"action":  "accept",
				"dstPort": "99999",
			},
		})
		require.ErrorContains(t, err, "out of range")
	})

	t.Run("valid", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{
				"server":  "321",
				"name":    "Allow SSH",
				"action":  "accept",
				"srcIp":   "0.0.0.0/0",
				"dstPort": "22",
			},
		})
		require.NoError(t, err)
	})
}

func Test_AddFirewallRule_Execute(t *testing.T) {
	component := &AddFirewallRule{}

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
						`{"firewall":{"server_number":"321","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow SSH","ip_version":"ipv4","protocol":"tcp","dst_port":"22","action":"accept"},{"name":"Allow HTTPS","ip_version":"ipv4","protocol":"tcp","dst_port":"443","action":"accept"}],"output":[]}}}`,
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
				"name":      "Allow HTTPS",
				"ipVersion": "ipv4",
				"protocol":  "tcp",
				"dstPort":   "443",
				"action":    "accept",
			},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, AddFirewallRulePayloadType, executionState.Type)
		require.Len(t, executionState.Payloads, 1)
		wrapped := executionState.Payloads[0].(map[string]any)
		payload := wrapped["data"].(map[string]any)
		assert.Equal(t, "321", payload["serverNumber"])
		assert.Equal(t, 2, payload["ruleCount"])
		rule := payload["rule"].(map[string]any)
		assert.Equal(t, "Allow HTTPS", rule["name"])
	})

	t.Run("duplicate_name", func(t *testing.T) {
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
				"name":   "Allow SSH",
				"action": "accept",
			},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.ErrorContains(t, err, "add firewall rule")
	})

	t.Run("get_fails", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"SERVER_ERROR","message":"internal error"}}`)),
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

		require.ErrorContains(t, err, "add firewall rule")
	})
}
