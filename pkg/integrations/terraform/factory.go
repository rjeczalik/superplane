package terraform

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tfplugin "github.com/superplanehq/superplane/pkg/integrations/terraform/plugin"
	protocolv5 "github.com/superplanehq/superplane/pkg/integrations/terraform/protocol/v5"
	protocolv6 "github.com/superplanehq/superplane/pkg/integrations/terraform/protocol/v6"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

type RuntimeFactory struct {
	launch func(ctx context.Context, protocolMajor int, req RuntimeLaunchRequest) (*launchedProvider, error)
}

type RuntimeLaunchRequest struct {
	Protocols        []string
	BinaryPath       string
	ExecutableSHA256 string
	ProviderName     string
	ProviderSource   string
	ProviderVersion  string
	OrgID            string
	Isolation        tfplugin.IsolationOptions
}

type launchedProvider struct {
	ProtocolMajor int
	Server        any
	Close         func()
}

func NewRuntimeFactory(launcher *tfplugin.PluginLauncher) *RuntimeFactory {
	return &RuntimeFactory{
		launch: func(ctx context.Context, protocolMajor int, req RuntimeLaunchRequest) (*launchedProvider, error) {
			if launcher == nil {
				return nil, fmt.Errorf("terraform plugin launcher is required")
			}
			plugin, err := launcher.LaunchBinary(ctx, req.BinaryPath, tfplugin.LaunchRequest{
				OrgID:            req.OrgID,
				ExecutableSHA256: req.ExecutableSHA256,
				ProtocolMajor:    protocolMajor,
				Isolation:        req.Isolation,
				ProviderName:     req.ProviderName,
				ProviderSource:   req.ProviderSource,
				ProviderVersion:  req.ProviderVersion,
			})
			if err != nil {
				return nil, err
			}
			return &launchedProvider{
				ProtocolMajor: plugin.ProtocolMajor,
				Server:        plugin.Server,
				Close:         plugin.Close,
			}, nil
		},
	}
}

func NewProviderRuntime(protocols []string, launcher *tfplugin.PluginLauncher, binaryPath string) (runtime.ProviderRuntime, error) {
	return NewRuntimeFactory(launcher).NewProviderRuntime(context.Background(), protocols, binaryPath)
}

func (f *RuntimeFactory) NewProviderRuntime(ctx context.Context, protocols []string, binaryPath string) (runtime.ProviderRuntime, error) {
	return f.NewProviderRuntimeForLaunch(ctx, RuntimeLaunchRequest{Protocols: protocols, BinaryPath: binaryPath})
}

func (f *RuntimeFactory) NewProviderRuntimeForLaunch(ctx context.Context, req RuntimeLaunchRequest) (runtime.ProviderRuntime, error) {
	selected, err := selectRuntimeProtocol(req.Protocols)
	if err != nil {
		return nil, err
	}
	if f == nil || f.launch == nil {
		return nil, fmt.Errorf("terraform runtime factory launch function is required")
	}

	launched, err := f.launch(ctx, selected, req)
	if err != nil {
		if shouldRetryProtocol6FromHandshake(err, selected) {
			selected = 6
			launched, err = f.launch(ctx, selected, req)
		}
		if err != nil {
			return nil, err
		}
	}
	if launched == nil {
		return nil, fmt.Errorf("terraform runtime factory launch returned nil provider")
	}
	if launched.ProtocolMajor != selected {
		if launched.Close != nil {
			launched.Close()
		}
		return nil, fmt.Errorf("terraform provider negotiated protocol %d but registry selected protocol %d", launched.ProtocolMajor, selected)
	}

	var adapter runtime.ProviderRuntime
	switch selected {
	case 6:
		server, ok := launched.Server.(tfprotov6.ProviderServer)
		if !ok {
			if launched.Close != nil {
				launched.Close()
			}
			return nil, fmt.Errorf("terraform provider protocol 6 client has unexpected type %T", launched.Server)
		}
		adapter = protocolv6.NewV6Adapter(server)
	case 5:
		server, ok := launched.Server.(tfprotov5.ProviderServer)
		if !ok {
			if launched.Close != nil {
				launched.Close()
			}
			return nil, fmt.Errorf("terraform provider protocol 5 client has unexpected type %T", launched.Server)
		}
		adapter = protocolv5.NewV5Adapter(server)
	default:
		if launched.Close != nil {
			launched.Close()
		}
		return nil, fmt.Errorf("unsupported Terraform plugin protocol %d", selected)
	}

	return &managedRuntime{ProviderRuntime: adapter, close: launched.Close}, nil
}

type managedRuntime struct {
	runtime.ProviderRuntime
	close func()
}

func (r *managedRuntime) Close() error {
	err := r.ProviderRuntime.Close()
	if r.close != nil {
		r.close()
	}
	return err
}

func selectRuntimeProtocol(protocols []string) (int, error) {
	if len(protocols) == 0 {
		return 0, &runtime.RegistryError{Kind: "protocol", Detail: "registry response does not advertise Terraform plugin protocols"}
	}

	best := 0
	for _, protocol := range protocols {
		major, _, ok := strings.Cut(protocol, ".")
		if !ok {
			continue
		}
		parsed, err := strconv.Atoi(major)
		if err != nil {
			continue
		}
		switch parsed {
		case 6:
			return 6, nil
		case 5:
			best = 5
		}
	}
	if best == 0 {
		return 0, &runtime.RegistryError{Kind: "protocol", Detail: "registry response does not advertise Terraform protocol v5 or v6"}
	}
	return best, nil
}

func shouldRetryProtocol6FromHandshake(err error, selected int) bool {
	if selected != 5 || err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "incompatible API version with plugin") &&
		strings.Contains(message, "Plugin version: 6") &&
		strings.Contains(message, "Client versions: [5]")
}
