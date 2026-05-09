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

const GetServerPayloadType = "hetznerRobot.server.fetched"

type GetServer struct{}
type GetServerSpec struct {
	Server string `json:"server" mapstructure:"server"`
}

var _ core.Action = (*GetServer)(nil)

func (g *GetServer) Name() string        { return "hetznerRobot.getServer" }
func (g *GetServer) Label() string       { return "Get Server" }
func (g *GetServer) Description() string { return "Fetch details of a Hetzner dedicated server" }
func (g *GetServer) Icon() string        { return "server" }
func (g *GetServer) Color() string       { return "gray" }
func (g *GetServer) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (g *GetServer) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:        "server",
			Label:       "Server",
			Type:        configuration.FieldTypeIntegrationResource,
			Required:    true,
			Description: "The dedicated server to retrieve",
			TypeOptions: &configuration.TypeOptions{
				Resource: &configuration.ResourceTypeOptions{Type: "server"},
			},
		},
	}
}
func (g *GetServer) Documentation() string {
	return `The Get Server component retrieves details about a Hetzner dedicated server.

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

func (g *GetServer) Setup(ctx core.SetupContext) error {
	spec := GetServerSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	return validateServerNumber(strings.TrimSpace(spec.Server))
}

func (g *GetServer) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (g *GetServer) Execute(ctx core.ExecutionContext) error {
	spec := GetServerSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	server, err := client.GetServer(serverNumber)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}

	payload := serverToPayload(server)
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, GetServerPayloadType, []any{payload})
}

func (g *GetServer) Hooks() []core.Hook                          { return []core.Hook{} }
func (g *GetServer) HandleHook(ctx core.ActionHookContext) error { return nil }
func (g *GetServer) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (g *GetServer) Cancel(ctx core.ExecutionContext) error { return nil }
func (g *GetServer) Cleanup(ctx core.SetupContext) error    { return nil }

func serverToPayload(s *Server) map[string]any {
	if s == nil {
		return map[string]any{}
	}
	out := map[string]any{
		"serverNumber": s.ServerNumber,
		"name":         s.DisplayName(),
		"product":      s.Product,
		"datacenter":   s.Datacenter,
		"status":       s.Status,
		"cancelled":    s.Cancelled,
	}
	if len(s.IP) > 0 && s.IP[0] != "" {
		out["ipv4"] = s.IP[0]
	}
	return out
}
