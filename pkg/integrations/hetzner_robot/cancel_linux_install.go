package hetznerrobot

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const CancelLinuxInstallPayloadType = "hetznerRobot.linux.cancelled"

type CancelLinuxInstall struct{}
type CancelLinuxInstallSpec struct {
	Server string `json:"server" mapstructure:"server"`
}

var _ core.Action = (*CancelLinuxInstall)(nil)

func (c *CancelLinuxInstall) Name() string  { return "hetznerRobot.cancelLinuxInstall" }
func (c *CancelLinuxInstall) Label() string { return "Cancel Linux Install" }
func (c *CancelLinuxInstall) Description() string {
	return "Cancel a pending Linux installation on a Hetzner dedicated server"
}
func (c *CancelLinuxInstall) Icon() string  { return "x-circle" }
func (c *CancelLinuxInstall) Color() string { return "red" }
func (c *CancelLinuxInstall) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (c *CancelLinuxInstall) Documentation() string {
	return `The Cancel Linux Install component deactivates a pending Linux installation on a Hetzner dedicated server.

If no installation is active (404), this is treated as a successful cancellation (idempotent).

## Fields

- **Server**: The dedicated server
`
}
func (c *CancelLinuxInstall) Configuration() []configuration.Field {
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

func (c *CancelLinuxInstall) Setup(ctx core.SetupContext) error {
	spec := CancelLinuxInstallSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	return validateServerNumber(strings.TrimSpace(spec.Server))
}

func (c *CancelLinuxInstall) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (c *CancelLinuxInstall) Execute(ctx core.ExecutionContext) error {
	spec := CancelLinuxInstallSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	if err := client.DeactivateLinux(serverNumber); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			// 404 = no active installation, treat as successful cancellation
		} else {
			return fmt.Errorf("cancel linux install: %w", err)
		}
	}

	payload := map[string]any{"serverNumber": serverNumber}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, CancelLinuxInstallPayloadType, []any{payload})
}

func (c *CancelLinuxInstall) Hooks() []core.Hook                          { return []core.Hook{} }
func (c *CancelLinuxInstall) HandleHook(ctx core.ActionHookContext) error { return nil }
func (c *CancelLinuxInstall) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (c *CancelLinuxInstall) Cancel(ctx core.ExecutionContext) error { return nil }
func (c *CancelLinuxInstall) Cleanup(ctx core.SetupContext) error    { return nil }
