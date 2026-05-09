package terraform

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tfplugin "github.com/superplanehq/superplane/pkg/integrations/terraform/plugin"
	protocolv5 "github.com/superplanehq/superplane/pkg/integrations/terraform/protocol/v5"
	protocolv6 "github.com/superplanehq/superplane/pkg/integrations/terraform/protocol/v6"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestSelectRuntimeProtocol(t *testing.T) {
	tests := []struct {
		name      string
		protocols []string
		want      int
		wantErr   bool
	}{
		{name: "v6 zero", protocols: []string{"6.0"}, want: 6},
		{name: "v6 minor", protocols: []string{"6.10"}, want: 6},
		{name: "v5 zero", protocols: []string{"5.0"}, want: 5},
		{name: "v5 minor", protocols: []string{"5.1"}, want: 5},
		{name: "prefers v6", protocols: []string{"5.0", "6.0"}, want: 6},
		{name: "unsupported", protocols: []string{"4.0"}, wantErr: true},
		{name: "empty", protocols: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectRuntimeProtocol(tt.protocols)
			if tt.wantErr {
				require.Error(t, err)
				var registryErr *runtime.RegistryError
				assert.ErrorAs(t, err, &registryErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeFactoryReturnsV6Adapter(t *testing.T) {
	factory := &RuntimeFactory{
		launch: func(_ context.Context, protocolMajor int, req RuntimeLaunchRequest) (*launchedProvider, error) {
			assert.Equal(t, 6, protocolMajor)
			assert.Equal(t, "/cached/provider", req.BinaryPath)
			return &launchedProvider{ProtocolMajor: 6, Server: &factoryV6Provider{}}, nil
		},
	}

	rt, err := factory.NewProviderRuntime(context.Background(), []string{"6.10"}, "/cached/provider")
	require.NoError(t, err)
	assert.IsType(t, &protocolv6.V6Adapter{}, rt.(*managedRuntime).ProviderRuntime)
}

func TestRuntimeFactoryReturnsV5Adapter(t *testing.T) {
	factory := &RuntimeFactory{
		launch: func(_ context.Context, protocolMajor int, _ RuntimeLaunchRequest) (*launchedProvider, error) {
			assert.Equal(t, 5, protocolMajor)
			return &launchedProvider{ProtocolMajor: 5, Server: &factoryV5Provider{}}, nil
		},
	}

	rt, err := factory.NewProviderRuntime(context.Background(), []string{"5.1"}, "/cached/provider")
	require.NoError(t, err)
	assert.IsType(t, &protocolv5.V5Adapter{}, rt.(*managedRuntime).ProviderRuntime)
}

func TestRuntimeFactoryFailsWhenNegotiatedProtocolDiffers(t *testing.T) {
	closed := false
	factory := &RuntimeFactory{
		launch: func(context.Context, int, RuntimeLaunchRequest) (*launchedProvider, error) {
			return &launchedProvider{ProtocolMajor: 5, Server: &factoryV5Provider{}, Close: func() { closed = true }}, nil
		},
	}

	_, err := factory.NewProviderRuntime(context.Background(), []string{"6.0"}, "/cached/provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negotiated protocol 5")
	assert.True(t, closed)
}

func TestRuntimeFactoryDoesNotFallbackToProtocol6OnGenericLaunchError(t *testing.T) {
	var requested []int
	factory := &RuntimeFactory{
		launch: func(_ context.Context, protocolMajor int, _ RuntimeLaunchRequest) (*launchedProvider, error) {
			requested = append(requested, protocolMajor)
			if protocolMajor == 5 {
				return nil, assert.AnError
			}
			return &launchedProvider{ProtocolMajor: 6, Server: &factoryV6Provider{}}, nil
		},
	}

	_, err := factory.NewProviderRuntime(context.Background(), []string{"5.0"}, "/cached/provider")
	require.Error(t, err)
	assert.Equal(t, []int{5}, requested)
}

func TestRuntimeFactoryFallsBackToProtocol6OnPluginHandshakeVersionMismatch(t *testing.T) {
	var requested []int
	factory := &RuntimeFactory{
		launch: func(_ context.Context, protocolMajor int, _ RuntimeLaunchRequest) (*launchedProvider, error) {
			requested = append(requested, protocolMajor)
			if protocolMajor == 5 {
				return nil, errors.New("plugin lifecycle error during handshake: incompatible API version with plugin. Plugin version: 6, Client versions: [5]")
			}
			return &launchedProvider{ProtocolMajor: 6, Server: &factoryV6Provider{}}, nil
		},
	}

	rt, err := factory.NewProviderRuntime(context.Background(), []string{"5.0"}, "/cached/provider")
	require.NoError(t, err)
	assert.IsType(t, &protocolv6.V6Adapter{}, rt.(*managedRuntime).ProviderRuntime)
	assert.Equal(t, []int{5, 6}, requested)
}

func TestRuntimeFactoryUsesPluginLauncher(t *testing.T) {
	cache, err := tfplugin.NewBinaryCache(t.TempDir(), 0)
	require.NoError(t, err)
	launcher, err := tfplugin.NewPluginLauncher(tfplugin.LauncherOptions{Cache: cache})
	require.NoError(t, err)

	_, err = NewRuntimeFactory(launcher).NewProviderRuntime(context.Background(), []string{"6.0"}, "/missing/provider")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "bridge is not implemented")
}

type factoryV5Provider struct{ tfprotov5.ProviderServer }

type factoryV6Provider struct{ tfprotov6.ProviderServer }
