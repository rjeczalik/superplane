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

const AddFirewallRulePayloadType = "hetznerRobot.firewallRule.added"

type AddFirewallRule struct{}
type AddFirewallRuleSpec struct {
	Server    string `json:"server" mapstructure:"server"`
	Name      string `json:"name" mapstructure:"name"`
	IPVersion string `json:"ipVersion" mapstructure:"ipVersion"`
	Protocol  string `json:"protocol" mapstructure:"protocol"`
	SrcIP     string `json:"srcIp" mapstructure:"srcIp"`
	DstPort   string `json:"dstPort" mapstructure:"dstPort"`
	SrcPort   string `json:"srcPort" mapstructure:"srcPort"`
	Action    string `json:"action" mapstructure:"action"`
	TCPFlags  string `json:"tcpFlags" mapstructure:"tcpFlags"`
}

var _ core.Action = (*AddFirewallRule)(nil)

func (a *AddFirewallRule) Name() string  { return "hetznerRobot.addFirewallRule" }
func (a *AddFirewallRule) Label() string { return "Add Firewall Rule" }
func (a *AddFirewallRule) Description() string {
	return "Add a firewall rule to a Hetzner dedicated server"
}
func (a *AddFirewallRule) Icon() string  { return "shield" }
func (a *AddFirewallRule) Color() string { return "green" }
func (a *AddFirewallRule) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (a *AddFirewallRule) Documentation() string {
	return `The Add Firewall Rule component adds a new inbound rule to a Hetzner dedicated server's firewall.

Uses read-modify-write: fetches current rules, appends the new rule, and applies the full set atomically.

## Fields

- **Server**: The dedicated server
- **Name**: Unique rule name (used as identifier for updates/deletes)
- **IP Version**: ipv4 or ipv6
- **Protocol**: tcp, udp, icmp, etc. (optional)
- **Source IP**: Source address in CIDR notation (optional)
- **Destination Port**: Port or range, e.g. 443 or 1000-2000 (optional)
- **Source Port**: Source port or range (optional)
- **Action**: accept or discard
- **TCP Flags**: TCP flags filter (optional)

## Output

- **serverNumber**: Server identifier
- **ruleCount**: Total rules after addition
- **rule**: The newly added rule
`
}
func (a *AddFirewallRule) Configuration() []configuration.Field {
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
		{
			Name:        "name",
			Label:       "Rule Name",
			Type:        configuration.FieldTypeString,
			Required:    true,
			Description: "Rule name",
		},
		{
			Name:        "ipVersion",
			Label:       "IP Version",
			Type:        configuration.FieldTypeSelect,
			Required:    true,
			Description: "IP version",
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "IPv4", Value: "ipv4"},
						{Label: "IPv6", Value: "ipv6"},
					},
				},
			},
		},
		{
			Name:        "protocol",
			Label:       "Protocol",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Description: "Protocol (tcp, udp, icmp, etc.)",
		},
		{
			Name:        "srcIp",
			Label:       "Source IP",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Description: "Source IP in CIDR notation",
		},
		{
			Name:        "dstPort",
			Label:       "Destination Port",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Description: "Destination port or range (e.g. 443 or 1000-2000)",
		},
		{
			Name:        "srcPort",
			Label:       "Source Port",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Description: "Source port or range",
		},
		{
			Name:        "action",
			Label:       "Action",
			Type:        configuration.FieldTypeSelect,
			Required:    true,
			Description: "Firewall action",
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "Accept", Value: "accept"},
						{Label: "Discard", Value: "discard"},
					},
				},
			},
		},
		{
			Name:        "tcpFlags",
			Label:       "TCP Flags",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Description: "TCP flags",
		},
	}
}

func (a *AddFirewallRule) Setup(ctx core.SetupContext) error {
	spec := AddFirewallRuleSpec{}
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
	if strings.TrimSpace(spec.Action) == "" {
		return fmt.Errorf("action is required")
	}
	if spec.IPVersion != "" && spec.IPVersion != "ipv4" && spec.IPVersion != "ipv6" {
		return fmt.Errorf("ipVersion must be ipv4 or ipv6")
	}
	if srcIP := strings.TrimSpace(spec.SrcIP); srcIP != "" {
		if err := validateCIDR(srcIP); err != nil {
			return err
		}
	}
	if dstPort := strings.TrimSpace(spec.DstPort); dstPort != "" {
		if err := validatePortOrRange(dstPort); err != nil {
			return err
		}
	}
	if srcPort := strings.TrimSpace(spec.SrcPort); srcPort != "" {
		if err := validatePortOrRange(srcPort); err != nil {
			return err
		}
	}
	return nil
}

func (a *AddFirewallRule) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func buildFirewallRule(spec AddFirewallRuleSpec) FirewallRule {
	ipVersion := strings.TrimSpace(spec.IPVersion)
	if ipVersion == "" {
		ipVersion = "ipv4"
	}
	return FirewallRule{
		Name:      strings.TrimSpace(spec.Name),
		IPVersion: ipVersion,
		Protocol:  strings.TrimSpace(spec.Protocol),
		SrcIP:     strings.TrimSpace(spec.SrcIP),
		DstPort:   strings.TrimSpace(spec.DstPort),
		SrcPort:   strings.TrimSpace(spec.SrcPort),
		Action:    strings.TrimSpace(spec.Action),
		TCPFlags:  strings.TrimSpace(spec.TCPFlags),
	}
}

func (a *AddFirewallRule) Execute(ctx core.ExecutionContext) error {
	spec := AddFirewallRuleSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	rule := buildFirewallRule(spec)
	result, err := client.AddFirewallRule(serverNumber, rule)
	if err != nil {
		return fmt.Errorf("add firewall rule: %w", err)
	}

	addedRule := map[string]any{
		"name":       rule.Name,
		"ip_version": rule.IPVersion,
		"action":     rule.Action,
	}
	if rule.Protocol != "" {
		addedRule["protocol"] = rule.Protocol
	}
	if rule.DstPort != "" {
		addedRule["dst_port"] = rule.DstPort
	}
	if rule.SrcIP != "" {
		addedRule["src_ip"] = rule.SrcIP
	}
	if rule.SrcPort != "" {
		addedRule["src_port"] = rule.SrcPort
	}
	if rule.TCPFlags != "" {
		addedRule["tcp_flags"] = rule.TCPFlags
	}

	payload := map[string]any{
		"serverNumber": serverNumber,
		"ruleCount":    len(result.Rules.Input),
		"rule":         addedRule,
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, AddFirewallRulePayloadType, []any{payload})
}

func (a *AddFirewallRule) Hooks() []core.Hook                          { return []core.Hook{} }
func (a *AddFirewallRule) HandleHook(ctx core.ActionHookContext) error { return nil }
func (a *AddFirewallRule) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (a *AddFirewallRule) Cancel(ctx core.ExecutionContext) error { return nil }
func (a *AddFirewallRule) Cleanup(ctx core.SetupContext) error    { return nil }
