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

const ResetServerPayloadType = "hetznerRobot.server.reset"

var validResetTypes = map[string]bool{"sw": true, "hw": true, "power": true, "power_long": true, "man": true}

type ResetServer struct{}
type ResetServerSpec struct {
	Server    string `json:"server" mapstructure:"server"`
	ResetType string `json:"resetType" mapstructure:"resetType"`
}

var _ core.Action = (*ResetServer)(nil)

func (r *ResetServer) Name() string  { return "hetznerRobot.resetServer" }
func (r *ResetServer) Label() string { return "Reset Server" }
func (r *ResetServer) Description() string {
	return "Execute a power reset on a Hetzner dedicated server and wait for completion"
}
func (r *ResetServer) Icon() string  { return "power" }
func (r *ResetServer) Color() string { return "orange" }
func (r *ResetServer) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (r *ResetServer) Documentation() string {
	return `The Reset Server component executes a power reset on a Hetzner dedicated server and polls until the reset completes.

## Configuration

- **Server**: The dedicated server to reset
- **Reset Type**: The type of reset to perform
  - **sw**: Software reset (CTRL+ALT+DEL)
  - **hw**: Hardware reset (power cycle)
  - **power**: Short power button press
  - **power_long**: Long power button press
  - **man**: Manual reset by datacenter technician

## Output

Returns the reset result including:
- **serverNumber**: Server ID
- **resetType**: The reset type that was executed
- **status**: Final reset status
`
}
func (r *ResetServer) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:        "server",
			Label:       "Server",
			Type:        configuration.FieldTypeIntegrationResource,
			Required:    true,
			Description: "The dedicated server to reset",
			TypeOptions: &configuration.TypeOptions{
				Resource: &configuration.ResourceTypeOptions{Type: "server"},
			},
		},
		{
			Name:        "resetType",
			Label:       "Reset Type",
			Type:        configuration.FieldTypeSelect,
			Required:    true,
			Description: "The type of reset to perform",
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "Software Reset (CTRL+ALT+DEL)", Value: "sw"},
						{Label: "Hardware Reset (power cycle)", Value: "hw"},
						{Label: "Power button press", Value: "power"},
						{Label: "Long power button press", Value: "power_long"},
						{Label: "Manual reset (technician)", Value: "man"},
					},
				},
			},
		},
	}
}

func (r *ResetServer) Setup(ctx core.SetupContext) error {
	spec := ResetServerSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	if err := validateServerNumber(strings.TrimSpace(spec.Server)); err != nil {
		return err
	}
	if strings.TrimSpace(spec.ResetType) == "" {
		return fmt.Errorf("resetType is required")
	}
	if !validResetTypes[spec.ResetType] {
		return fmt.Errorf("invalid resetType %q: must be one of sw, hw, power, power_long, man", spec.ResetType)
	}
	return nil
}

func (r *ResetServer) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (r *ResetServer) Execute(ctx core.ExecutionContext) error {
	spec := ResetServerSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)
	resetType := strings.TrimSpace(spec.ResetType)

	if !validResetTypes[resetType] {
		return fmt.Errorf("invalid resetType %q", resetType)
	}

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	_, err = client.ResetServer(serverNumber, resetType)
	if err != nil {
		return fmt.Errorf("reset server: %w", err)
	}

	payload := map[string]any{
		"serverNumber": serverNumber,
		"resetType":    resetType,
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, ResetServerPayloadType, []any{payload})
}

func (r *ResetServer) Hooks() []core.Hook                          { return []core.Hook{} }
func (r *ResetServer) HandleHook(ctx core.ActionHookContext) error { return nil }

func (r *ResetServer) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (r *ResetServer) Cancel(ctx core.ExecutionContext) error { return nil }
func (r *ResetServer) Cleanup(ctx core.SetupContext) error    { return nil }
