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

const EnableRescuePayloadType = "hetznerRobot.rescue.enabled"

type EnableRescue struct{}
type EnableRescueSpec struct {
	Server         string   `json:"server" mapstructure:"server"`
	OS             string   `json:"os" mapstructure:"os"`
	AuthorizedKeys []string `json:"authorizedKeys" mapstructure:"authorizedKeys"`
}

var _ core.Action = (*EnableRescue)(nil)

func (e *EnableRescue) Name() string  { return "hetznerRobot.enableRescue" }
func (e *EnableRescue) Label() string { return "Enable Rescue" }
func (e *EnableRescue) Description() string {
	return "Enable rescue boot mode on a Hetzner dedicated server"
}
func (e *EnableRescue) Icon() string  { return "shield" }
func (e *EnableRescue) Color() string { return "blue" }
func (e *EnableRescue) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (e *EnableRescue) Documentation() string {
	return `The Enable Rescue component activates rescue boot mode on a Hetzner dedicated server.

After enabling, the server must be reset to boot into rescue mode.
A root password is generated and stored securely via integration secrets.

## Fields

- **Server**: The dedicated server
- **OS**: Rescue operating system (linux, linuxold, freebsd, freebsdold, vkvm, vkvmold)
- **SSH Keys**: Optional list of SSH key fingerprints for authorized access

**Note:** Architecture is fixed to 64-bit. 32-bit rescue images are not supported.
`
}
func (e *EnableRescue) Configuration() []configuration.Field {
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
			Name:        "os",
			Label:       "Rescue OS",
			Type:        configuration.FieldTypeSelect,
			Required:    true,
			Description: "Rescue operating system",
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "Linux", Value: "linux"},
						{Label: "Linux Old", Value: "linuxold"},
						{Label: "FreeBSD", Value: "freebsd"},
						{Label: "FreeBSD Old", Value: "freebsdold"},
						{Label: "vKVM", Value: "vkvm"},
						{Label: "vKVM Old", Value: "vkvmold"},
					},
				},
			},
		},
		{
			Name:        "authorizedKeys",
			Label:       "SSH Keys",
			Type:        configuration.FieldTypeList,
			Required:    false,
			Description: "SSH key fingerprints for rescue access",
			TypeOptions: &configuration.TypeOptions{
				List: &configuration.ListTypeOptions{
					ItemLabel: "SSH Key Fingerprint",
					ItemDefinition: &configuration.ListItemDefinition{
						Type: configuration.FieldTypeString,
					},
				},
			},
		},
	}
}

func (e *EnableRescue) Setup(ctx core.SetupContext) error {
	spec := EnableRescueSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	if err := validateServerNumber(strings.TrimSpace(spec.Server)); err != nil {
		return err
	}
	if strings.TrimSpace(spec.OS) == "" {
		return fmt.Errorf("os is required")
	}
	return nil
}

func (e *EnableRescue) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (e *EnableRescue) Execute(ctx core.ExecutionContext) error {
	spec := EnableRescueSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	config, err := client.EnableRescue(serverNumber, strings.TrimSpace(spec.OS), "64", spec.AuthorizedKeys)
	if err != nil {
		return fmt.Errorf("enable rescue: %w", err)
	}

	secretName := fmt.Sprintf("rescue-password-%s", serverNumber)
	if config.Password != "" {
		if err := ctx.Integration.SetSecret(secretName, []byte(config.Password)); err != nil {
			return fmt.Errorf("failed to store rescue password: %w", err)
		}
		config.Password = ""
	}

	payload := map[string]any{
		"serverNumber":       serverNumber,
		"os":                 config.OS,
		"passwordGenerated":  true,
		"passwordSecretName": secretName,
	}
	if len(config.AuthorizedKey) > 0 {
		payload["authorizedKeys"] = config.AuthorizedKey
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, EnableRescuePayloadType, []any{payload})
}

func (e *EnableRescue) Hooks() []core.Hook                          { return []core.Hook{} }
func (e *EnableRescue) HandleHook(ctx core.ActionHookContext) error { return nil }
func (e *EnableRescue) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (e *EnableRescue) Cancel(ctx core.ExecutionContext) error { return nil }
func (e *EnableRescue) Cleanup(ctx core.SetupContext) error    { return nil }
