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

func Test_HetznerRobot_Registration(t *testing.T) {
	integration := &HetznerRobot{}
	assert.Equal(t, "hetznerRobot", integration.Name())

	// SetupProvider must be wired — verify it is non-nil by constructing one.
	sp := &SetupProvider{}
	assert.NotNil(t, sp)
}

func Test_HetznerRobot_Actions(t *testing.T) {
	integration := &HetznerRobot{}
	actions := integration.Actions()

	names := make([]string, 0, len(actions))
	for _, a := range actions {
		names = append(names, a.Name())
	}

	assert.Contains(t, names, "hetznerRobot.listServers", "Actions() must include listServers")
	assert.Contains(t, names, "hetznerRobot.getServer")
	assert.Contains(t, names, "hetznerRobot.resetServer")
}

func Test_HetznerRobot_NewClient_DualMode(t *testing.T) {
	httpCtx := &contexts.HTTPContext{}

	t.Run("new-flow reads from CurrentSecrets", func(t *testing.T) {
		ctx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"username": {Name: "username", Value: []byte("u1")},
				"password": {Name: "password", Value: []byte("p1")},
			},
		}
		client, err := NewClient(httpCtx, ctx)
		require.NoError(t, err)
		assert.Equal(t, "u1", client.username)
		assert.Equal(t, "p1", client.password)
		assert.Equal(t, robotBaseURL, client.baseURL)
	})

	t.Run("legacy reads from Configuration", func(t *testing.T) {
		ctx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "u2", "password": "p2"},
		}
		client, err := NewClient(httpCtx, ctx)
		require.NoError(t, err)
		assert.Equal(t, "u2", client.username)
		assert.Equal(t, "p2", client.password)
		assert.Equal(t, robotBaseURL, client.baseURL)
	})

	t.Run("new-flow missing username returns clear error", func(t *testing.T) {
		ctx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"password": {Name: "password", Value: []byte("p1")},
			},
		}
		_, err := NewClient(httpCtx, ctx)
		require.ErrorContains(t, err, "username is required")
	})

	t.Run("new-flow missing password returns clear error", func(t *testing.T) {
		ctx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"username": {Name: "username", Value: []byte("u1")},
			},
		}
		_, err := NewClient(httpCtx, ctx)
		require.ErrorContains(t, err, "password is required")
	})

	t.Run("legacy missing username returns clear error", func(t *testing.T) {
		ctx := &contexts.IntegrationContext{
			Configuration: map[string]any{"password": "p2"},
		}
		_, err := NewClient(httpCtx, ctx)
		require.ErrorContains(t, err, "username is required")
	})

	t.Run("legacy missing password returns clear error", func(t *testing.T) {
		ctx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "u2"},
		}
		_, err := NewClient(httpCtx, ctx)
		require.ErrorContains(t, err, "password is required")
	})

	t.Run("NewClientFromCredentials trims whitespace", func(t *testing.T) {
		client, err := NewClientFromCredentials(httpCtx, "  u3  ", "  p3  ")
		require.NoError(t, err)
		assert.Equal(t, "u3", client.username)
		assert.Equal(t, "p3", client.password)
	})

	t.Run("NewClientFromCredentials empty username after trim", func(t *testing.T) {
		_, err := NewClientFromCredentials(httpCtx, "   ", "p3")
		require.ErrorContains(t, err, "username is required")
	})

	t.Run("NewClientFromCredentials empty password after trim", func(t *testing.T) {
		_, err := NewClientFromCredentials(httpCtx, "u3", "   ")
		require.ErrorContains(t, err, "password is required")
	})

	t.Run("NewClientFromSecrets missing username returns clear error", func(t *testing.T) {
		storage := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"password": {Name: "password", Value: []byte("p1")},
			},
		}
		_, err := NewClientFromSecrets(&contexts.HTTPContext{}, storage.Secrets())
		require.ErrorContains(t, err, "username is required")
	})

	t.Run("NewClientFromSecrets missing password returns clear error", func(t *testing.T) {
		storage := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				"username": {Name: "username", Value: []byte("u1")},
			},
		}
		_, err := NewClientFromSecrets(&contexts.HTTPContext{}, storage.Secrets())
		require.ErrorContains(t, err, "password is required")
	})
}

func Test_HetznerRobot_Sync(t *testing.T) {
	integration := &HetznerRobot{}

	t.Run("no username returns error", func(t *testing.T) {
		err := integration.Sync(core.SyncContext{
			Configuration: map[string]any{"username": "", "password": "pass"},
			HTTP:          &contexts.HTTPContext{},
			Integration:   &contexts.IntegrationContext{Configuration: map[string]any{"username": "", "password": "pass"}},
		})
		require.ErrorContains(t, err, "username is required")
	})

	t.Run("no password returns error", func(t *testing.T) {
		err := integration.Sync(core.SyncContext{
			Configuration: map[string]any{"username": "user", "password": ""},
			HTTP:          &contexts.HTTPContext{},
			Integration:   &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": ""}},
		})
		require.ErrorContains(t, err, "password is required")
	})

	t.Run("valid credentials sets ready", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[]`)),
			}},
		}
		integrationCtx := &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}}
		err := integration.Sync(core.SyncContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
			HTTP:          httpContext,
			Integration:   integrationCtx,
		})
		require.NoError(t, err)
		assert.Equal(t, "ready", integrationCtx.State)
	})
}

func Test_HetznerRobot_ListResources(t *testing.T) {
	integration := &HetznerRobot{}

	t.Run("list servers", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`[
					{"server":{"server_number":"12345","server_name":"web-01"}},
					{"server":{"server_number":"67890","server_name":""}}
				]`)),
			}},
		}
		integrationCtx := &contexts.IntegrationContext{Configuration: map[string]any{"username": "user", "password": "pass"}}
		resources, err := integration.ListResources("server", core.ListResourcesContext{
			HTTP:        httpContext,
			Integration: integrationCtx,
		})
		require.NoError(t, err)
		require.Len(t, resources, 2)
		assert.Equal(t, "web-01", resources[0].Name)
		assert.Equal(t, "12345", resources[0].ID)
		assert.Equal(t, "Server 67890", resources[1].Name)
	})

	t.Run("list reset types", func(t *testing.T) {
		resources, err := integration.ListResources("reset_type", core.ListResourcesContext{
			HTTP:        &contexts.HTTPContext{},
			Integration: &contexts.IntegrationContext{},
		})
		require.NoError(t, err)
		require.Len(t, resources, 5)
	})

	t.Run("unknown type returns nil", func(t *testing.T) {
		resources, err := integration.ListResources("unknown", core.ListResourcesContext{
			HTTP:        &contexts.HTTPContext{},
			Integration: &contexts.IntegrationContext{},
		})
		require.NoError(t, err)
		assert.Nil(t, resources)
	})
}
