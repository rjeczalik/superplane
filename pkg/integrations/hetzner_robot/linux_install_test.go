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

func Test_InstallLinux_Setup(t *testing.T) {
	component := &InstallLinux{}

	t.Run("missing_server", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"dist": "Debian 12 base"},
		})
		require.ErrorContains(t, err, "server is required")
	})

	t.Run("missing_dist", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "321"},
		})
		require.ErrorContains(t, err, "dist is required")
	})

	t.Run("valid", func(t *testing.T) {
		err := component.Setup(core.SetupContext{
			Configuration: map[string]any{"server": "321", "dist": "Debian 12 base"},
		})
		require.NoError(t, err)
	})
}

func linuxConfigResponse(dists ...string) *http.Response {
	items := make([]string, len(dists))
	for i, d := range dists {
		items[i] = `"` + d + `"`
	}
	body := `{"linux":{"server_number":"321","dist":[` + strings.Join(items, ",") + `],"lang":["en","de","fi"]}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func Test_InstallLinux_Execute(t *testing.T) {
	component := &InstallLinux{}

	t.Run("success", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				linuxConfigResponse("Debian 12 base", "Ubuntu 22.04.1 LTS minimal"),
				{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"linux":{"server_number":"321","dist":"Debian 12 base","lang":"en","active":true,"password":"rootpw123","authorized_key":["ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89"],"host_key":["ecdsa-sha2-nistp256 AAAA..."]}}`,
					)),
				},
			},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321", "dist": "Debian 12 base", "lang": "en"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
		assert.Equal(t, "default", executionState.Channel)
		assert.Equal(t, InstallLinuxPayloadType, executionState.Type)
		secret, ok := integrationCtx.CurrentSecrets["linux-password-321"]
		require.True(t, ok, "linux-password-321 secret should be set")
		assert.Equal(t, []byte("rootpw123"), secret.Value)
	})

	t.Run("invalid_dist_returns_error_with_available", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				linuxConfigResponse("Debian 12 base", "Ubuntu 22.04.1 LTS minimal"),
			},
		}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321", "dist": "Ubuntu 22.04"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: &contexts.ExecutionStateContext{KVs: map[string]string{}},
		})

		require.ErrorContains(t, err, `invalid distribution "Ubuntu 22.04"`)
		require.ErrorContains(t, err, "Debian 12 base")
		require.ErrorContains(t, err, "Ubuntu 22.04.1 LTS minimal")
	})

	t.Run("default_lang", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				linuxConfigResponse("Debian 12 base"),
				{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"linux":{"server_number":"321","dist":"Debian 12 base","lang":"en","active":true,"password":"","authorized_key":[],"host_key":[]}}`,
					)),
				},
			},
		}
		executionState := &contexts.ExecutionStateContext{KVs: map[string]string{}}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321", "dist": "Debian 12 base"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: executionState,
		})

		require.NoError(t, err)
		assert.True(t, executionState.Passed)
	})

	t.Run("api_error_on_activate", func(t *testing.T) {
		httpContext := &contexts.HTTPContext{
			Responses: []*http.Response{
				linuxConfigResponse("Debian 12 base"),
				{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"SERVER_ERROR","message":"internal error"}}`)),
				},
			},
		}
		integrationCtx := &contexts.IntegrationContext{
			Configuration: map[string]any{"username": "user", "password": "pass"},
		}

		err := component.Execute(core.ExecutionContext{
			Configuration:  map[string]any{"server": "321", "dist": "Debian 12 base"},
			HTTP:           httpContext,
			Integration:    integrationCtx,
			ExecutionState: &contexts.ExecutionStateContext{KVs: map[string]string{}},
		})

		require.ErrorContains(t, err, "install linux")
	})
}
