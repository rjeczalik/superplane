package hetznerrobot

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const ListSSHKeysPayloadType = "hetznerRobot.sshKey.listed"

type ListSSHKeys struct{}

var _ core.Action = (*ListSSHKeys)(nil)

func (l *ListSSHKeys) Name() string        { return "hetznerRobot.listSshKeys" }
func (l *ListSSHKeys) Label() string       { return "List SSH Keys" }
func (l *ListSSHKeys) Description() string { return "List all SSH keys in the Hetzner Robot account" }
func (l *ListSSHKeys) Icon() string        { return "key" }
func (l *ListSSHKeys) Color() string       { return "gray" }
func (l *ListSSHKeys) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}
func (l *ListSSHKeys) Documentation() string {
	return `The List SSH Keys component retrieves all SSH keys stored in the Hetzner Robot account.

## Output

- **keys**: Array of SSH keys with name, fingerprint, type, and size
- **keyCount**: Total number of keys
`
}
func (l *ListSSHKeys) Configuration() []configuration.Field {
	return []configuration.Field{}
}

func (l *ListSSHKeys) Setup(ctx core.SetupContext) error {
	return nil
}

func (l *ListSSHKeys) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (l *ListSSHKeys) Execute(ctx core.ExecutionContext) error {
	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}

	keys, err := client.ListSSHKeys()
	if err != nil {
		return fmt.Errorf("list ssh keys: %w", err)
	}

	keyList := make([]map[string]any, len(keys))
	for i, k := range keys {
		keyList[i] = map[string]any{
			"name":        k.Name,
			"fingerprint": k.Fingerprint,
			"type":        k.Type,
			"size":        k.Size,
		}
	}

	payload := map[string]any{
		"keys":     keyList,
		"keyCount": len(keys),
	}
	return ctx.ExecutionState.Emit(core.DefaultOutputChannel.Name, ListSSHKeysPayloadType, []any{payload})
}

func (l *ListSSHKeys) Hooks() []core.Hook                          { return []core.Hook{} }
func (l *ListSSHKeys) HandleHook(ctx core.ActionHookContext) error { return nil }
func (l *ListSSHKeys) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}
func (l *ListSSHKeys) Cancel(ctx core.ExecutionContext) error { return nil }
func (l *ListSSHKeys) Cleanup(ctx core.SetupContext) error    { return nil }
