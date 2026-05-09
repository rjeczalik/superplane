package hetznerrobot

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const WakeOnLANPayloadType = "hetznerRobot.server.woken"

type WakeOnLAN struct{}
type WakeOnLANSpec struct {
	Server string `json:"server" mapstructure:"server"`
}

var _ core.Action = (*WakeOnLAN)(nil)

func (w *WakeOnLAN) Name() string  { return "hetznerRobot.wakeOnLan" }
func (w *WakeOnLAN) Label() string { return "Wake on LAN" }
func (w *WakeOnLAN) Description() string {
	return "Send a Wake-on-LAN packet to a Hetzner dedicated server"
}
func (w *WakeOnLAN) Icon() string  { return "server" }
func (w *WakeOnLAN) Color() string { return "gray" }
func (w *WakeOnLAN) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (w *WakeOnLAN) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:        "server",
			Label:       "Server",
			Type:        configuration.FieldTypeIntegrationResource,
			Required:    true,
			Description: "The dedicated server to wake",
			TypeOptions: &configuration.TypeOptions{
				Resource: &configuration.ResourceTypeOptions{Type: "server"},
			},
		},
	}
}
func (w *WakeOnLAN) Documentation() string {
	return `The Wake on LAN component sends a Wake-on-LAN magic packet to a Hetzner dedicated server.

## Output

Returns:
- **serverNumber**: Server ID
- **serverIP**: Primary IPv4 address
`
}

func (w *WakeOnLAN) Setup(ctx core.SetupContext) error {
	spec := WakeOnLANSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	return nil
}

func (w *WakeOnLAN) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (w *WakeOnLAN) Execute(ctx core.ExecutionContext) error {
	spec := WakeOnLANSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	result, err := client.SendWOL(serverNumber)
	if err != nil {
		return fmt.Errorf("wake on lan: %w", err)
	}

	payload := map[string]any{
		"serverNumber": result.ServerNumber,
		"serverIP":     result.ServerIP,
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, WakeOnLANPayloadType, []any{payload})
}

func (w *WakeOnLAN) Hooks() []core.Hook                          { return []core.Hook{} }
func (w *WakeOnLAN) HandleHook(ctx core.ActionHookContext) error { return nil }
func (w *WakeOnLAN) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (w *WakeOnLAN) Cancel(ctx core.ExecutionContext) error { return nil }
func (w *WakeOnLAN) Cleanup(ctx core.SetupContext) error    { return nil }
