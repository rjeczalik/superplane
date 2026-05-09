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

const DeleteFirewallRulePayloadType = "hetznerRobot.firewallRule.deleted"

type DeleteFirewallRule struct{}
type DeleteFirewallRuleSpec struct {
	Server string `json:"server" mapstructure:"server"`
	Name   string `json:"name" mapstructure:"name"`
}

var _ core.Action = (*DeleteFirewallRule)(nil)

func (d *DeleteFirewallRule) Name() string  { return "hetznerRobot.deleteFirewallRule" }
func (d *DeleteFirewallRule) Label() string { return "Delete Firewall Rule" }
func (d *DeleteFirewallRule) Description() string {
	return "Delete a firewall rule from a Hetzner dedicated server"
}
func (d *DeleteFirewallRule) Icon() string  { return "shield" }
func (d *DeleteFirewallRule) Color() string { return "red" }
func (d *DeleteFirewallRule) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (d *DeleteFirewallRule) Documentation() string {
	return `The Delete Firewall Rule component removes an inbound rule from a Hetzner dedicated server's firewall.

Matches the rule by name, removes it, and applies the remaining rules atomically.

## Fields

- **Server**: The dedicated server
- **Name**: Name of the rule to delete (must exist)

## Output

- **serverNumber**: Server identifier
- **ruleCount**: Remaining rules after deletion
- **deletedRuleName**: Name of the removed rule
`
}
func (d *DeleteFirewallRule) Configuration() []configuration.Field {
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
			Description: "Rule name to delete",
		},
	}
}

func (d *DeleteFirewallRule) Setup(ctx core.SetupContext) error {
	spec := DeleteFirewallRuleSpec{}
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

func (d *DeleteFirewallRule) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (d *DeleteFirewallRule) Execute(ctx core.ExecutionContext) error {
	spec := DeleteFirewallRuleSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)
	ruleName := strings.TrimSpace(spec.Name)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	result, err := client.DeleteFirewallRuleByName(serverNumber, ruleName)
	if err != nil {
		return fmt.Errorf("delete firewall rule: %w", err)
	}

	payload := map[string]any{
		"serverNumber":    serverNumber,
		"ruleCount":       len(result.Rules.Input),
		"deletedRuleName": ruleName,
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, DeleteFirewallRulePayloadType, []any{payload})
}

func (d *DeleteFirewallRule) Hooks() []core.Hook                          { return []core.Hook{} }
func (d *DeleteFirewallRule) HandleHook(ctx core.ActionHookContext) error { return nil }
func (d *DeleteFirewallRule) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (d *DeleteFirewallRule) Cancel(ctx core.ExecutionContext) error { return nil }
func (d *DeleteFirewallRule) Cleanup(ctx core.SetupContext) error    { return nil }
