package hetznerrobot

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const InstallLinuxPayloadType = "hetznerRobot.linux.installed"

type InstallLinux struct{}
type InstallLinuxSpec struct {
	Server         string   `json:"server" mapstructure:"server"`
	Dist           string   `json:"dist" mapstructure:"dist"`
	Lang           string   `json:"lang" mapstructure:"lang"`
	AuthorizedKeys []string `json:"authorizedKeys" mapstructure:"authorizedKeys"`
}

var _ core.Action = (*InstallLinux)(nil)

func (i *InstallLinux) Name() string  { return "hetznerRobot.installLinux" }
func (i *InstallLinux) Label() string { return "Install Linux" }
func (i *InstallLinux) Description() string {
	return "Activate Linux installation on a Hetzner dedicated server"
}
func (i *InstallLinux) Icon() string  { return "hard-drive" }
func (i *InstallLinux) Color() string { return "blue" }
func (i *InstallLinux) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (i *InstallLinux) Documentation() string {
	return `The Install Linux component activates a Linux installation on a Hetzner dedicated server.

The server must be rebooted after activation for installation to begin.
A root password is generated and stored securely via integration secrets.

## Fields

- **Server**: The dedicated server
- **Distribution**: Linux distribution to install
- **Language**: Installation language (en, de, fi)
- **SSH Keys**: Optional SSH key fingerprints for authorized access
`
}
func (i *InstallLinux) Configuration() []configuration.Field {
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
			Name:        "dist",
			Label:       "Distribution",
			Type:        configuration.FieldTypeString,
			Required:    true,
			Description: "Linux distribution to install",
		},
		{
			Name:        "lang",
			Label:       "Language",
			Type:        configuration.FieldTypeSelect,
			Required:    false,
			Description: "Installation language (defaults to en)",
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "English", Value: "en"},
						{Label: "German", Value: "de"},
						{Label: "Finnish", Value: "fi"},
					},
				},
			},
		},
		{
			Name:        "authorizedKeys",
			Label:       "SSH Keys",
			Type:        configuration.FieldTypeList,
			Required:    false,
			Description: "SSH key fingerprints for authorized access",
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

func (i *InstallLinux) Setup(ctx core.SetupContext) error {
	spec := InstallLinuxSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Server) == "" {
		return fmt.Errorf("server is required")
	}
	if err := validateServerNumber(strings.TrimSpace(spec.Server)); err != nil {
		return err
	}
	if strings.TrimSpace(spec.Dist) == "" {
		return fmt.Errorf("dist is required")
	}
	return nil
}

func (i *InstallLinux) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (i *InstallLinux) Execute(ctx core.ExecutionContext) error {
	spec := InstallLinuxSpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	serverNumber := strings.TrimSpace(spec.Server)
	lang := strings.TrimSpace(spec.Lang)
	if lang == "" {
		lang = "en"
	}

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	dist := strings.TrimSpace(spec.Dist)
	linuxConfig, err := client.GetLinuxConfig(serverNumber)
	if err != nil {
		return fmt.Errorf("fetch available distributions: %w", err)
	}
	if !slices.Contains(linuxConfig.Dist, dist) {
		return fmt.Errorf("invalid distribution %q for server %s; available: %s",
			dist, serverNumber, strings.Join(linuxConfig.Dist, ", "))
	}

	config, err := client.ActivateLinux(serverNumber, dist, lang, spec.AuthorizedKeys)
	if err != nil {
		return fmt.Errorf("install linux: %w", err)
	}

	secretName := fmt.Sprintf("linux-password-%s", serverNumber)
	if config.Password != "" {
		if err := ctx.Integration.SetSecret(secretName, []byte(config.Password)); err != nil {
			return fmt.Errorf("failed to store linux password: %w", err)
		}
		config.Password = ""
	}

	payload := map[string]any{
		"serverNumber":       serverNumber,
		"dist":               config.Dist,
		"lang":               config.Lang,
		"passwordGenerated":  true,
		"passwordSecretName": secretName,
	}
	if len(config.AuthorizedKey) > 0 {
		payload["authorizedKeys"] = config.AuthorizedKey
	}
	if len(config.HostKey) > 0 {
		payload["hostKeys"] = config.HostKey
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, InstallLinuxPayloadType, []any{payload})
}

func (i *InstallLinux) Hooks() []core.Hook                          { return []core.Hook{} }
func (i *InstallLinux) HandleHook(ctx core.ActionHookContext) error { return nil }
func (i *InstallLinux) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (i *InstallLinux) Cancel(ctx core.ExecutionContext) error { return nil }
func (i *InstallLinux) Cleanup(ctx core.SetupContext) error    { return nil }
