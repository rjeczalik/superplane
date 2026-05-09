package terraform

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/database"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/gorm"
)

type GeneratedResourceTrigger struct {
	integrationName string
	resourceName    string
	resourceType    string
	icon            string
}

func NewGeneratedResourceTrigger(integrationName, resourceName, resourceType string, icon ...string) *GeneratedResourceTrigger {
	t := &GeneratedResourceTrigger{integrationName: integrationName, resourceName: resourceName, resourceType: resourceType}
	if len(icon) > 0 {
		t.icon = icon[0]
	}
	return t
}

func (t *GeneratedResourceTrigger) Name() string {
	return fmt.Sprintf("%s.%s.onChanged", t.integrationName, t.resourceName)
}

func (t *GeneratedResourceTrigger) Label() string {
	return "On " + strings.Title(strings.Join(splitCamelCase(t.resourceName), " ")) + " Changed"
}

func (t *GeneratedResourceTrigger) Description() string         { return "" }
func (t *GeneratedResourceTrigger) Documentation() string       { return "# " + t.Label() }
func (t *GeneratedResourceTrigger) Icon() string                { return t.icon }
func (t *GeneratedResourceTrigger) Color() string               { return "" }
func (t *GeneratedResourceTrigger) ExampleData() map[string]any { return nil }
func (t *GeneratedResourceTrigger) Hooks() []core.Hook          { return nil }
func (t *GeneratedResourceTrigger) HandleHook(ctx core.TriggerHookContext) (map[string]any, error) {
	return nil, nil
}
func (t *GeneratedResourceTrigger) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}

func (t *GeneratedResourceTrigger) Configuration() []configuration.Field {
	return []configuration.Field{
		{Name: "poll_interval_secs", Label: "Poll Interval", Type: configuration.FieldTypeNumber, Default: 300},
		{Name: "managed_resource_id", Label: "Managed Resource ID", Type: configuration.FieldTypeString},
		{Name: "idempotency_key", Label: "Idempotency Key", Type: configuration.FieldTypeString},
		{Name: "changed_fields", Label: "Changed Fields", Type: configuration.FieldTypeList},
	}
}

func (t *GeneratedResourceTrigger) Setup(ctx core.TriggerContext) error {
	orgID, canvasID, integrationID, cfg, err := t.contextInputs(ctx)
	if err != nil {
		return err
	}
	return database.Conn().Transaction(func(tx *gorm.DB) error {
		_, err := models.UpsertSubscriptionForNode(tx, orgID, canvasID, integrationID, ctx.NodeID, t.resourceType, cfg)
		return err
	})
}

func (t *GeneratedResourceTrigger) Cleanup(ctx core.TriggerContext) error {
	canvasID, err := uuid.Parse(ctx.WorkflowID)
	if err != nil {
		return err
	}
	return models.DisableSubscriptionForNode(database.Conn(), canvasID, ctx.NodeID, t.resourceType)
}

func (t *GeneratedResourceTrigger) contextInputs(ctx core.TriggerContext) (uuid.UUID, uuid.UUID, uuid.UUID, models.TerraformManagedResourceSubscriptionConfig, error) {
	orgID, err := uuid.Parse(ctx.OrganizationID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, models.TerraformManagedResourceSubscriptionConfig{}, err
	}
	canvasID, err := uuid.Parse(ctx.WorkflowID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, models.TerraformManagedResourceSubscriptionConfig{}, err
	}
	if ctx.Integration == nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, models.TerraformManagedResourceSubscriptionConfig{}, fmt.Errorf("integration context required")
	}
	cfgMap, err := mapConfig(ctx.Configuration)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, models.TerraformManagedResourceSubscriptionConfig{}, err
	}
	cfg := models.TerraformManagedResourceSubscriptionConfig{Enabled: true, PollIntervalSecs: 300}
	if raw, ok := cfgMap["poll_interval_secs"].(float64); ok {
		cfg.PollIntervalSecs = int(raw)
	}
	if raw, ok := cfgMap["managed_resource_id"].(string); ok && raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, models.TerraformManagedResourceSubscriptionConfig{}, err
		}
		cfg.ManagedResourceID = &parsed
	}
	if raw, ok := cfgMap["idempotency_key"].(string); ok && raw != "" {
		cfg.IdempotencyKey = &raw
	}
	if raw, ok := cfgMap["changed_fields"]; ok {
		fields, err := stringListFromConfigValue(raw)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, models.TerraformManagedResourceSubscriptionConfig{}, err
		}
		cfg.ChangedFields = fields
	}
	return orgID, canvasID, ctx.Integration.ID(), cfg, nil
}

func stringListFromConfigValue(value any) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []string:
		return typed, nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("changed_fields must be a list of strings")
			}
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("changed_fields must be a list of strings")
	}
}
