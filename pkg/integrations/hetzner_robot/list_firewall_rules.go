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

const ListFirewallRulesPayloadType = "hetznerRobot.firewallRule.listed"

type ListFirewallRules struct{}
type ListFirewallRulesSpec struct {
	Server string `json:"server" mapstructure:"server"`
}

var _ core.Action = (*ListFirewallRules)(nil)

func (l *ListFirewallRules) Name() string  { return "hetznerRobot.listFirewallRules" }
func (l *ListFirewallRules) Label() string { return "List Firewall Rules" }
func (l *ListFirewallRules) Description() string {
	return "List all firewall rules on a Hetzner dedicated server"
}
func (l *ListFirewallRules) Icon() string  { return "shield" }
func (l *ListFirewallRules) Color() string { return "gray" }
func (l *ListFirewallRules) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (l *ListFirewallRules) Documentation() string {
	return `The List Firewall Rules component retrieves the current firewall configuration for a Hetzner dedicated server.

## Fields

- **Server**: The dedicated server

## Output

- **serverNumber**: Server identifier
- **status**: Firewall status (active, disabled, in process)
- **whitelistHos**: Whether Hetzner services are whitelisted
- **rules**: Array of input rules with name, ip_version, protocol, ports, source IP, and action
- **ruleCount**: Total number of input rules
`
}
func (l *ListFirewallRules) Configuration() []configuration.Field {
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

func (l *ListFirewallRules) Setup(ctx core.SetupContext) error {
	spec := ListFirewallRulesSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	return validateServerNumber(strings.TrimSpace(spec.Server))
}

func (l *ListFirewallRules) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (l *ListFirewallRules) Execute(ctx core.ExecutionContext) error {
	spec := ListFirewallRulesSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	config, err := client.GetFirewall(serverNumber)
	if err != nil {
		return fmt.Errorf("get firewall: %w", err)
	}

	rules := make([]any, 0, len(config.Rules.Input))
	for _, r := range config.Rules.Input {
		rule := map[string]any{}
		if r.Name != "" {
			rule["name"] = r.Name
		}
		if r.IPVersion != "" {
			rule["ip_version"] = r.IPVersion
		}
		if r.Protocol != "" {
			rule["protocol"] = r.Protocol
		}
		if r.DstPort != "" {
			rule["dst_port"] = r.DstPort
		}
		if r.SrcIP != "" {
			rule["src_ip"] = r.SrcIP
		}
		if r.DstIP != "" {
			rule["dst_ip"] = r.DstIP
		}
		if r.Action != "" {
			rule["action"] = r.Action
		}
		if r.TCPFlags != "" {
			rule["tcp_flags"] = r.TCPFlags
		}
		rules = append(rules, rule)
	}

	payload := map[string]any{
		"serverNumber": serverNumber,
		"status":       config.Status,
		"whitelistHos": config.WhitelistHos,
		"rules":        rules,
		"ruleCount":    len(config.Rules.Input),
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, ListFirewallRulesPayloadType, []any{payload})
}

func (l *ListFirewallRules) Hooks() []core.Hook                          { return []core.Hook{} }
func (l *ListFirewallRules) HandleHook(ctx core.ActionHookContext) error { return nil }
func (l *ListFirewallRules) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (l *ListFirewallRules) Cancel(ctx core.ExecutionContext) error { return nil }
func (l *ListFirewallRules) Cleanup(ctx core.SetupContext) error    { return nil }
