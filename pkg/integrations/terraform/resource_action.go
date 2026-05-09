package terraform

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

type GeneratedResourceAction struct {
	base   *GeneratedAction
	op     string
	runner *ResourceRunner
}

func NewGeneratedResourceAction(base *GeneratedAction, op string, runner *ResourceRunner) *GeneratedResourceAction {
	copy := *base
	copy.op = op
	return &GeneratedResourceAction{base: &copy, op: op, runner: runner}
}

func (a *GeneratedResourceAction) Name() string { return a.base.Name() }

func (a *GeneratedResourceAction) SchemaHash() string { return a.base.SchemaHash() }

func (a *GeneratedResourceAction) Label() string {
	label := strings.Title(strings.Join(splitCamelCase(a.base.resourceName), " "))
	switch a.op {
	case "create":
		return "Create " + label
	case "read":
		return "Read " + label
	case "update":
		return "Update " + label
	case "delete":
		return "Delete " + label
	default:
		return label
	}
}

func (a *GeneratedResourceAction) Description() string           { return a.base.Description() }
func (a *GeneratedResourceAction) Documentation() string         { return a.base.Documentation() }
func (a *GeneratedResourceAction) Icon() string                  { return a.base.Icon() }
func (a *GeneratedResourceAction) Color() string                 { return a.base.Color() }
func (a *GeneratedResourceAction) ExampleOutput() map[string]any { return a.base.ExampleOutput() }
func (a *GeneratedResourceAction) OutputChannels(configuration any) []core.OutputChannel {
	return a.base.OutputChannels(configuration)
}
func (a *GeneratedResourceAction) Setup(ctx core.SetupContext) error { return nil }
func (a *GeneratedResourceAction) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}
func (a *GeneratedResourceAction) Hooks() []core.Hook { return nil }
func (a *GeneratedResourceAction) HandleHook(ctx core.ActionHookContext) error {
	return nil
}
func (a *GeneratedResourceAction) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return 0, nil, nil
}
func (a *GeneratedResourceAction) Cancel(ctx core.ExecutionContext) error { return nil }
func (a *GeneratedResourceAction) Cleanup(ctx core.SetupContext) error    { return nil }

func (a *GeneratedResourceAction) Configuration() []configuration.Field {
	idField := configuration.Field{Name: "managed_resource_id", Label: "Managed Resource ID", Type: configuration.FieldTypeString, Required: true}
	switch a.op {
	case "create":
		fields := append([]configuration.Field{}, a.base.inputSchema...)
		fields = append(fields,
			configuration.Field{Name: "idempotency_key", Label: "Idempotency Key", Type: configuration.FieldTypeString},
			configuration.Field{Name: "on_existing", Label: "On Existing", Type: configuration.FieldTypeSelect, Default: "fail"},
		)
		return fields
	case "read":
		return []configuration.Field{idField}
	case "update":
		fields := []configuration.Field{idField}
		for _, field := range a.base.inputSchema {
			field.Required = false
			fields = append(fields, field)
		}
		fields = append(fields, configuration.Field{Name: "replacement_strategy", Label: "Replacement Strategy", Type: configuration.FieldTypeSelect, Default: "fail"})
		return fields
	case "delete":
		return []configuration.Field{
			idField,
			{Name: "confirm_delete", Label: "Confirm Delete", Type: configuration.FieldTypeBool, Required: true},
			{Name: "force_forget", Label: "Force Forget", Type: configuration.FieldTypeBool},
			{Name: "confirm_forget", Label: "Confirm Forget", Type: configuration.FieldTypeBool},
		}
	default:
		return nil
	}
}

func (a *GeneratedResourceAction) Execute(ctx core.ExecutionContext) error {
	if a.runner == nil {
		return fmt.Errorf("terraform resource runner not configured")
	}
	switch a.op {
	case "create":
		return a.runner.Create(ctx, a.base)
	case "read":
		return a.runner.Read(ctx, a.base)
	case "update":
		return a.runner.Update(ctx, a.base)
	case "delete":
		return a.runner.Delete(ctx, a.base)
	default:
		return fmt.Errorf("unsupported terraform resource operation %q", a.op)
	}
}
