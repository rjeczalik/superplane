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

const DisableRescuePayloadType = "hetznerRobot.rescue.disabled"

type DisableRescue struct{}
type DisableRescueSpec struct {
	Server string `json:"server" mapstructure:"server"`
}

var _ core.Action = (*DisableRescue)(nil)

func (d *DisableRescue) Name() string  { return "hetznerRobot.disableRescue" }
func (d *DisableRescue) Label() string { return "Disable Rescue" }
func (d *DisableRescue) Description() string {
	return "Disable rescue boot mode on a Hetzner dedicated server"
}
func (d *DisableRescue) Icon() string  { return "shield-off" }
func (d *DisableRescue) Color() string { return "gray" }
func (d *DisableRescue) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (d *DisableRescue) Documentation() string {
	return `The Disable Rescue component deactivates rescue boot mode on a Hetzner dedicated server.

## Fields

- **Server**: The dedicated server
`
}
func (d *DisableRescue) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:        "server",
			Label:       "Server",
			Type:        configuration.FieldTypeIntegrationResource,
			Required:    true,
			Description: "The dedicated server",
			TypeOptions: &configuration.TypeOptions{
				Resource: &configuration.ResourceTypeOptions{Type: "server"},
			},
		},
	}
}

func (d *DisableRescue) Setup(ctx core.SetupContext) error {
	spec := DisableRescueSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	return validateServerNumber(strings.TrimSpace(spec.Server))
}

func (d *DisableRescue) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (d *DisableRescue) Execute(ctx core.ExecutionContext) error {
	spec := DisableRescueSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	if err := client.DisableRescue(serverNumber); err != nil {
		return fmt.Errorf("disable rescue: %w", err)
	}

	payload := map[string]any{"serverNumber": serverNumber}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, DisableRescuePayloadType, []any{payload})
}

func (d *DisableRescue) Hooks() []core.Hook                          { return []core.Hook{} }
func (d *DisableRescue) HandleHook(ctx core.ActionHookContext) error { return nil }
func (d *DisableRescue) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (d *DisableRescue) Cancel(ctx core.ExecutionContext) error { return nil }
func (d *DisableRescue) Cleanup(ctx core.SetupContext) error    { return nil }
