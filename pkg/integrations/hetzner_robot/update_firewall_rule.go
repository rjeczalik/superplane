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

const UpdateFirewallRulePayloadType = "hetznerRobot.firewallRule.updated"

type UpdateFirewallRule struct{}
type UpdateFirewallRuleSpec struct {
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

var _ core.Action = (*UpdateFirewallRule)(nil)

func (u *UpdateFirewallRule) Name() string  { return "hetznerRobot.updateFirewallRule" }
func (u *UpdateFirewallRule) Label() string { return "Update Firewall Rule" }
func (u *UpdateFirewallRule) Description() string {
	return "Update a firewall rule on a Hetzner dedicated server"
}
func (u *UpdateFirewallRule) Icon() string  { return "shield" }
func (u *UpdateFirewallRule) Color() string { return "blue" }
func (u *UpdateFirewallRule) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (u *UpdateFirewallRule) Documentation() string {
	return `The Update Firewall Rule component replaces an existing inbound rule on a Hetzner dedicated server's firewall.

Matches the existing rule by name, replaces it with the new definition, and applies atomically.

## Fields

- **Server**: The dedicated server
- **Name**: Name of the rule to update (must exist)
- **IP Version**: ipv4 or ipv6
- **Protocol**: tcp, udp, icmp, etc. (optional)
- **Source IP**: Source address in CIDR notation (optional)
- **Destination Port**: Port or range (optional)
- **Source Port**: Source port or range (optional)
- **Action**: accept or discard
- **TCP Flags**: TCP flags filter (optional)

## Output

- **serverNumber**: Server identifier
- **ruleCount**: Total rules after update
- **rule**: The updated rule
`
}
func (u *UpdateFirewallRule) Configuration() []configuration.Field {
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

func (u *UpdateFirewallRule) Setup(ctx core.SetupContext) error {
	spec := UpdateFirewallRuleSpec{}
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

func (u *UpdateFirewallRule) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (u *UpdateFirewallRule) Execute(ctx core.ExecutionContext) error {
	spec := UpdateFirewallRuleSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)
	ruleName := strings.TrimSpace(spec.Name)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	ipVersion := strings.TrimSpace(spec.IPVersion)
	if ipVersion == "" {
		ipVersion = "ipv4"
	}
	rule := FirewallRule{
		Name:      ruleName,
		IPVersion: ipVersion,
		Protocol:  strings.TrimSpace(spec.Protocol),
		SrcIP:     strings.TrimSpace(spec.SrcIP),
		DstPort:   strings.TrimSpace(spec.DstPort),
		SrcPort:   strings.TrimSpace(spec.SrcPort),
		Action:    strings.TrimSpace(spec.Action),
		TCPFlags:  strings.TrimSpace(spec.TCPFlags),
	}

	result, err := client.UpdateFirewallRule(serverNumber, ruleName, rule)
	if err != nil {
		return fmt.Errorf("update firewall rule: %w", err)
	}

	updatedRule := map[string]any{
		"name":       rule.Name,
		"ip_version": rule.IPVersion,
		"action":     rule.Action,
	}
	if rule.Protocol != "" {
		updatedRule["protocol"] = rule.Protocol
	}
	if rule.DstPort != "" {
		updatedRule["dst_port"] = rule.DstPort
	}
	if rule.SrcIP != "" {
		updatedRule["src_ip"] = rule.SrcIP
	}
	if rule.SrcPort != "" {
		updatedRule["src_port"] = rule.SrcPort
	}
	if rule.TCPFlags != "" {
		updatedRule["tcp_flags"] = rule.TCPFlags
	}

	payload := map[string]any{
		"serverNumber": serverNumber,
		"ruleCount":    len(result.Rules.Input),
		"rule":         updatedRule,
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, UpdateFirewallRulePayloadType, []any{payload})
}

func (u *UpdateFirewallRule) Hooks() []core.Hook                          { return []core.Hook{} }
func (u *UpdateFirewallRule) HandleHook(ctx core.ActionHookContext) error { return nil }
func (u *UpdateFirewallRule) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (u *UpdateFirewallRule) Cancel(ctx core.ExecutionContext) error { return nil }
func (u *UpdateFirewallRule) Cleanup(ctx core.SetupContext) error    { return nil }
