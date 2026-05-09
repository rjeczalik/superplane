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

const RenameServerPayloadType = "hetznerRobot.server.renamed"

type RenameServer struct{}
type RenameServerSpec struct {
	Server string `json:"server" mapstructure:"server"`
	Name   string `json:"name" mapstructure:"name"`
}

var _ core.Action = (*RenameServer)(nil)

func (r *RenameServer) Name() string        { return "hetznerRobot.renameServer" }
func (r *RenameServer) Label() string       { return "Rename Server" }
func (r *RenameServer) Description() string { return "Rename a Hetzner dedicated server" }
func (r *RenameServer) Icon() string        { return "server" }
func (r *RenameServer) Color() string       { return "gray" }
func (r *RenameServer) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (r *RenameServer) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:        "server",
			Label:       "Server",
			Type:        configuration.FieldTypeIntegrationResource,
			Required:    true,
			Description: "The dedicated server to rename",
			TypeOptions: &configuration.TypeOptions{
				Resource: &configuration.ResourceTypeOptions{Type: "server"},
			},
		},
		{
			Name:        "name",
			Label:       "Name",
			Type:        configuration.FieldTypeString,
			Required:    true,
			Description: "New display name for the server",
		},
	}
}
func (r *RenameServer) Documentation() string {
	return `The Rename Server component changes the display name of a Hetzner dedicated server.

## Output

Returns server details including:
- **serverNumber**: Server ID
- **name**: Server hostname
- **product**: Server product type
- **datacenter**: Datacenter location
- **status**: Server status
- **cancelled**: Whether the server is cancelled
- **ipv4**: Primary IPv4 address
`
}

func (r *RenameServer) Setup(ctx core.SetupContext) error {
	spec := RenameServerSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	if err := validateServerNumber(strings.TrimSpace(spec.Server)); err != nil {
		return err
	}
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

func (r *RenameServer) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (r *RenameServer) Execute(ctx core.ExecutionContext) error {
	spec := RenameServerSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	server, err := client.RenameServer(serverNumber, strings.TrimSpace(spec.Name))
	if err != nil {
		return fmt.Errorf("rename server: %w", err)
	}

	payload := serverToPayload(server)
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, RenameServerPayloadType, []any{payload})
}

func (r *RenameServer) Hooks() []core.Hook                          { return []core.Hook{} }
func (r *RenameServer) HandleHook(ctx core.ActionHookContext) error { return nil }
func (r *RenameServer) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (r *RenameServer) Cancel(ctx core.ExecutionContext) error { return nil }
func (r *RenameServer) Cleanup(ctx core.SetupContext) error    { return nil }
