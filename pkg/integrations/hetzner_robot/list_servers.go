package hetznerrobot

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const ListServersPayloadType = "hetznerRobot.server.listed"

type ListServers struct{}

var _ core.Action = (*ListServers)(nil)

func (l *ListServers) Name() string  { return "hetznerRobot.listServers" }
func (l *ListServers) Label() string { return "List Servers" }
func (l *ListServers) Description() string {
	return "List all dedicated servers in the Hetzner Robot account"
}
func (l *ListServers) Icon() string  { return "server" }
func (l *ListServers) Color() string { return "gray" }
func (l *ListServers) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (l *ListServers) Configuration() []configuration.Field {
	return []configuration.Field{}
}

func (l *ListServers) Documentation() string {
	return `The List Servers component retrieves all dedicated servers in the Hetzner Robot account.

## Output

- **servers**: Array of server objects, each containing:
  - **serverNumber**: Server ID
  - **name**: Server hostname
  - **product**: Server product type
  - **datacenter**: Datacenter location
  - **status**: Server status
  - **cancelled**: Whether the server is cancelled
  - **ipv4**: Primary IPv4 address (if available)
- **serverCount**: Total number of servers
`
}

func (l *ListServers) Setup(ctx core.SetupContext) error {
	return nil
}

func (l *ListServers) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (l *ListServers) Execute(ctx core.ExecutionContext) error {
	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	servers, err := client.ListServers()
	if err != nil {
		return fmt.Errorf("list servers: %w", err)
	}

	serverList := make([]map[string]any, len(servers))
	for i, s := range servers {
		serverList[i] = serverToPayload(&s)
	}

	payload := map[string]any{
		"servers":     serverList,
		"serverCount": len(servers),
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, ListServersPayloadType, []any{payload})
}

func (l *ListServers) Hooks() []core.Hook                          { return []core.Hook{} }
func (l *ListServers) HandleHook(ctx core.ActionHookContext) error { return nil }
func (l *ListServers) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (l *ListServers) Cancel(ctx core.ExecutionContext) error { return nil }
func (l *ListServers) Cleanup(ctx core.SetupContext) error    { return nil }
