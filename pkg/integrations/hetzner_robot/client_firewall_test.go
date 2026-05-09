package hetznerrobot

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/test/support/contexts"
)

const (
	firewallGetOneRule  = `{"firewall":{"server_number":"123","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow SSH","ip_version":"ipv4","protocol":"tcp","dst_port":"22","src_ip":"0.0.0.0/0","action":"accept"}],"output":[]}}}`
	firewallGetTwoRules = `{"firewall":{"server_number":"123","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow SSH","ip_version":"ipv4","protocol":"tcp","dst_port":"22","src_ip":"0.0.0.0/0","action":"accept"},{"name":"Allow HTTPS","ip_version":"ipv4","protocol":"tcp","dst_port":"443","action":"accept"}],"output":[]}}}`
	firewallGetNoRules  = `{"firewall":{"server_number":"123","status":"active","whitelist_hos":true,"rules":{"input":[],"output":[]}}}`
	firewallError500    = `{"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`
)

func newTestClient(httpCtx *contexts.HTTPContext) *Client {
	return &Client{
		http:     httpCtx,
		baseURL:  "https://robot-ws.your-server.de",
		username: "user",
		password: "pass",
	}
}

func Test_AddFirewallRule_Success(t *testing.T) {
	t.Run("adds rule to existing firewall rules", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(firewallGetOneRule)),
				},
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(firewallGetTwoRules)),
				},
			},
		}

		client := newTestClient(httpContext)
		result, err := client.AddFirewallRule("123", FirewallRule{
			Name:      "Allow HTTPS",
			IPVersion: "ipv4",
			Protocol:  "tcp",
			DstPort:   "443",
			Action:    "accept",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Rules.Input, 2)
		assert.Equal(t, "Allow SSH", result.Rules.Input[0].Name)
		assert.Equal(t, "Allow HTTPS", result.Rules.Input[1].Name)
		assert.Len(t, httpContext.Requests, 2)
		assert.Equal(t, "GET", httpContext.Requests[0].Method)
		assert.Equal(t, "POST", httpContext.Requests[1].Method)
	})
}

func Test_AddFirewallRule_DuplicateName(t *testing.T) {
	t.Run("returns error when rule with same name already exists", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(firewallGetOneRule)),
				},
			},
		}

		client := newTestClient(httpContext)
		result, err := client.AddFirewallRule("123", FirewallRule{
			Name:    "Allow SSH",
			Action:  "accept",
			DstPort: "22",
		})

		require.ErrorContains(t, err, `firewall rule "Allow SSH" already exists`)
		assert.Nil(t, result)
		// Only GET should have been called, no POST
		assert.Len(t, httpContext.Requests, 1)
		assert.Equal(t, "GET", httpContext.Requests[0].Method)
	})
}

func Test_AddFirewallRule_GetFails(t *testing.T) {
	t.Run("propagates error when GET firewall fails", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader(firewallError500)),
				},
			},
		}

		client := newTestClient(httpContext)
		result, err := client.AddFirewallRule("123", FirewallRule{
			Name:   "Allow HTTPS",
			Action: "accept",
		})

		require.ErrorContains(t, err, "get current firewall")
		assert.Nil(t, result)
		assert.Len(t, httpContext.Requests, 1)
	})
}

func Test_UpdateFirewallRule_Success(t *testing.T) {
	t.Run("updates matching rule in place", func(t *testing.T) {
		updatedFirewall := `{"firewall":{"server_number":"123","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow SSH","ip_version":"ipv4","protocol":"tcp","dst_port":"2222","src_ip":"0.0.0.0/0","action":"accept"}],"output":[]}}}`
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(firewallGetOneRule)),
				},
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(updatedFirewall)),
				},
			},
		}

		client := newTestClient(httpContext)
		result, err := client.UpdateFirewallRule("123", "Allow SSH", FirewallRule{
			Name:      "Allow SSH",
			IPVersion: "ipv4",
			Protocol:  "tcp",
			DstPort:   "2222",
			SrcIP:     "0.0.0.0/0",
			Action:    "accept",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Rules.Input, 1)
		assert.Equal(t, "2222", result.Rules.Input[0].DstPort)
		assert.Len(t, httpContext.Requests, 2)
		assert.Equal(t, "GET", httpContext.Requests[0].Method)
		assert.Equal(t, "POST", httpContext.Requests[1].Method)
	})
}

func Test_UpdateFirewallRule_NotFound(t *testing.T) {
	t.Run("returns error when rule name does not exist", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(firewallGetOneRule)),
				},
			},
		}

		client := newTestClient(httpContext)
		result, err := client.UpdateFirewallRule("123", "Allow HTTPS", FirewallRule{
			Name:   "Allow HTTPS",
			Action: "accept",
		})

		require.ErrorContains(t, err, `firewall rule "Allow HTTPS" not found`)
		assert.Nil(t, result)
		// Only GET, no POST
		assert.Len(t, httpContext.Requests, 1)
		assert.Equal(t, "GET", httpContext.Requests[0].Method)
	})
}

func Test_DeleteFirewallRuleByName_Success(t *testing.T) {
	t.Run("removes named rule and returns updated firewall", func(t *testing.T) {
		afterDelete := `{"firewall":{"server_number":"123","status":"active","whitelist_hos":true,"rules":{"input":[{"name":"Allow HTTPS","ip_version":"ipv4","protocol":"tcp","dst_port":"443","action":"accept"}],"output":[]}}}`
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(firewallGetTwoRules)),
				},
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(afterDelete)),
				},
			},
		}

		client := newTestClient(httpContext)
		result, err := client.DeleteFirewallRuleByName("123", "Allow SSH")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Rules.Input, 1)
		assert.Equal(t, "Allow HTTPS", result.Rules.Input[0].Name)
		assert.Len(t, httpContext.Requests, 2)
		assert.Equal(t, "GET", httpContext.Requests[0].Method)
		assert.Equal(t, "POST", httpContext.Requests[1].Method)
	})
}

func Test_DeleteFirewallRuleByName_NotFound(t *testing.T) {
	t.Run("returns error when rule name does not exist", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(firewallGetOneRule)),
				},
			},
		}

		client := newTestClient(httpContext)
		result, err := client.DeleteFirewallRuleByName("123", "Allow HTTPS")

		require.ErrorContains(t, err, `firewall rule "Allow HTTPS" not found`)
		assert.Nil(t, result)
		// Only GET, no POST
		assert.Len(t, httpContext.Requests, 1)
		assert.Equal(t, "GET", httpContext.Requests[0].Method)
	})
}
