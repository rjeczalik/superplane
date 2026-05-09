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

const AddSSHKeyPayloadType = "hetznerRobot.sshKey.added"

type AddSSHKey struct{}
type AddSSHKeySpec struct {
	Name string `json:"name" mapstructure:"name"`
	Data string `json:"data" mapstructure:"data"`
}

var _ core.Action = (*AddSSHKey)(nil)

var validSSHKeyPrefixes = []string{"ssh-rsa", "ssh-ed25519", "ecdsa-sha2-nistp", "ssh-dss"}

func hasValidSSHKeyPrefix(data string) bool {
	for _, prefix := range validSSHKeyPrefixes {
		if strings.HasPrefix(data, prefix) {
			return true
		}
	}
	return false
}

func (a *AddSSHKey) Name() string        { return "hetznerRobot.addSshKey" }
func (a *AddSSHKey) Label() string       { return "Add SSH Key" }
func (a *AddSSHKey) Description() string { return "Add a new SSH key to the Hetzner Robot account" }
func (a *AddSSHKey) Icon() string        { return "key" }
func (a *AddSSHKey) Color() string       { return "green" }
func (a *AddSSHKey) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (a *AddSSHKey) Documentation() string {
	return `The Add SSH Key component uploads a new SSH public key to the Hetzner Robot account.

## Fields

- **Name**: A label for the SSH key
- **Data**: The SSH public key data (ssh-rsa, ssh-ed25519, ecdsa, or ssh-dss format)

## Output

- **name**: Key name
- **fingerprint**: MD5 fingerprint of the key
- **type**: Key type (RSA, ED25519, etc.)
- **size**: Key size in bits
`
}
func (a *AddSSHKey) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:        "name",
			Label:       "Name",
			Type:        configuration.FieldTypeString,
			Required:    true,
			Description: "Name for the SSH key",
		},
		{
			Name:        "data",
			Label:       "Public Key",
			Type:        configuration.FieldTypeText,
			Required:    true,
			Description: "SSH public key data",
		},
	}
}

func (a *AddSSHKey) Setup(ctx core.SetupContext) error {
	spec := AddSSHKeySpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(spec.Data) == "" {
		return fmt.Errorf("data is required")
	}
	if len(spec.Data) >= 16384 {
		return fmt.Errorf("data size must be less than 16KB")
	}
	if !hasValidSSHKeyPrefix(strings.TrimSpace(spec.Data)) {
		return fmt.Errorf("data must start with a recognized SSH key prefix (ssh-rsa, ssh-ed25519, ecdsa-sha2-nistp, or ssh-dss)")
	}
	return nil
}

func (a *AddSSHKey) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (a *AddSSHKey) Execute(ctx core.ExecutionContext) error {
	spec := AddSSHKeySpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	key, err := client.AddSSHKey(strings.TrimSpace(spec.Name), strings.TrimSpace(spec.Data))
	if err != nil {
		return fmt.Errorf("add ssh key: %w", err)
	}

	payload := map[string]any{
		"name":        key.Name,
		"fingerprint": key.Fingerprint,
		"type":        key.Type,
		"size":        key.Size,
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, AddSSHKeyPayloadType, []any{payload})
}

func (a *AddSSHKey) Hooks() []core.Hook                          { return []core.Hook{} }
func (a *AddSSHKey) HandleHook(ctx core.ActionHookContext) error { return nil }
func (a *AddSSHKey) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (a *AddSSHKey) Cancel(ctx core.ExecutionContext) error { return nil }
func (a *AddSSHKey) Cleanup(ctx core.SetupContext) error    { return nil }
