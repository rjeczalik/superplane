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

const DeleteSSHKeyPayloadType = "hetznerRobot.sshKey.deleted"

type DeleteSSHKey struct{}
type DeleteSSHKeySpec struct {
	Fingerprint string `json:"fingerprint" mapstructure:"fingerprint"`
}

var _ core.Action = (*DeleteSSHKey)(nil)

func (d *DeleteSSHKey) Name() string  { return "hetznerRobot.deleteSshKey" }
func (d *DeleteSSHKey) Label() string { return "Delete SSH Key" }
func (d *DeleteSSHKey) Description() string {
	return "Delete an SSH key from the Hetzner Robot account"
}
func (d *DeleteSSHKey) Icon() string  { return "key" }
func (d *DeleteSSHKey) Color() string { return "red" }
func (d *DeleteSSHKey) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (d *DeleteSSHKey) Documentation() string {
	return `The Delete SSH Key component removes an SSH key from the Hetzner Robot account.

## Fields

- **Fingerprint**: The SSH key to delete (selected from account keys)

## Output

- **fingerprint**: The fingerprint of the deleted key
`
}
func (d *DeleteSSHKey) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:        "fingerprint",
			Label:       "SSH Key",
			Type:        configuration.FieldTypeIntegrationResource,
			Required:    true,
			Description: "SSH key to delete",
			TypeOptions: &configuration.TypeOptions{
				Resource: &configuration.ResourceTypeOptions{Type: "ssh_key"},
			},
		},
	}
}

func (d *DeleteSSHKey) Setup(ctx core.SetupContext) error {
	spec := DeleteSSHKeySpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(spec.Fingerprint) == "" {
		return fmt.Errorf("fingerprint is required")
	}
	return validateFingerprint(strings.TrimSpace(spec.Fingerprint))
}

func (d *DeleteSSHKey) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (d *DeleteSSHKey) Execute(ctx core.ExecutionContext) error {
	spec := DeleteSSHKeySpec{}
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	fingerprint := strings.TrimSpace(spec.Fingerprint)

	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	if err := client.DeleteSSHKey(fingerprint); err != nil {
		return fmt.Errorf("delete ssh key: %w", err)
	}

	payload := map[string]any{"fingerprint": fingerprint}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, DeleteSSHKeyPayloadType, []any{payload})
}

func (d *DeleteSSHKey) Hooks() []core.Hook                          { return []core.Hook{} }
func (d *DeleteSSHKey) HandleHook(ctx core.ActionHookContext) error { return nil }
func (d *DeleteSSHKey) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (d *DeleteSSHKey) Cancel(ctx core.ExecutionContext) error { return nil }
func (d *DeleteSSHKey) Cleanup(ctx core.SetupContext) error    { return nil }
