package terraform

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/sirupsen/logrus"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

type DroppedCapability struct {
	Name   string
	Reason string
}

func BuildIntegration(
	cfg config.TerraformProviderIntegration,
	schemas ProviderSchemasFile,
	validator TerraformValidator,
	runner ActionRunner,
	resourceRunner *ResourceRunner,
	logger *logrus.Entry,
) (*GeneratedIntegration, []DroppedCapability, error) {
	provider, ok := schemas.ProviderSchemas[cfg.Source]
	if !ok {
		return nil, nil, fmt.Errorf("provider schema %q not found", cfg.Source)
	}
	providerName := cfg.Name
	integrationName := TerraformIntegrationName(providerName)

	providerConfigSchema, droppedProviderConfig, err := translateProviderConfig(provider.Provider.Block)
	if err != nil {
		return nil, nil, err
	}

	var dropped []DroppedCapability
	dropped = append(dropped, droppedProviderConfig...)

	var actions []core.Action
	mappedActions, mappedTriggers, _, mappedDropped, err := (&CapabilityMapper{}).Map(capabilitySchemaFromProvider(provider), cfg, runner, resourceRunner)
	if err != nil {
		return nil, nil, err
	}
	actions = append(actions, mappedActions...)
	dropped = append(dropped, mappedDropped...)

	groups := capabilityGroupsForActions(actions)
	groups = append(groups, capabilityGroupsForTriggers(mappedTriggers)...)

	setupProvider := &GeneratedSetupProvider{
		integrationName:      integrationName,
		providerName:         providerName,
		capabilityGroups:     groups,
		providerConfigSchema: providerConfigSchema,
		providerSource:       cfg.Source,
		providerVersion:      cfg.Version,
		validator:            validator,
	}

	integration := &GeneratedIntegration{
		name:                 integrationName,
		label:                cfg.Label,
		icon:                 cfg.Icon,
		description:          cfg.Description,
		providerName:         providerName,
		providerSource:       cfg.Source,
		providerVersion:      cfg.Version,
		providerConfigSchema: providerConfigSchema,
		actions:              actions,
		triggers:             mappedTriggers,
		setupProvider:        setupProvider,
		validator:            validator,
	}

	if logger != nil {
		for _, d := range dropped {
			logger.WithField("capability", d.Name).Warn(d.Reason)
		}
	}

	return integration, dropped, nil
}

func buildAction(cfg config.TerraformProviderIntegration, terraformName, op string, schema SchemaRepresentation, runner ActionRunner) (*GeneratedAction, core.Capability, bool) {
	translated, ok := TranslateBlock(schema.Block)
	if !ok {
		return nil, core.Capability{}, false
	}

	resourceName := terraformResourceName(cfg.Name, terraformName)
	integrationName := TerraformIntegrationName(cfg.Name)
	action := &GeneratedAction{
		integrationName: integrationName,
		resourceName:    resourceName,
		op:              op,
		description:     schema.Block.Description,
		icon:            cfg.Icon,
		inputSchema:     translated.InputFields,
		outputSchema:    translated.OutputFields,
		sensitiveAttrs:  translated.SensitiveAttrs,
		capabilityName:  fmt.Sprintf("%s.%s.%s", integrationName, resourceName, op),
		schemaHash:      translated.SchemaHash,
		providerName:    cfg.Name,
		providerSource:  cfg.Source,
		providerVersion: cfg.Version,
		runner:          runner,
	}

	capability := core.Capability{
		Type:           core.IntegrationCapabilityTypeAction,
		Name:           action.Name(),
		Label:          action.Label(),
		Description:    action.Description(),
		Configuration:  action.Configuration(),
		OutputChannels: action.OutputChannels(nil),
	}

	return action, capability, true
}

func capabilitiesForOperation(actions []core.Action, op string, kind string) []core.Capability {
	capabilities := []core.Capability{}
	for _, action := range actions {
		switch generated := action.(type) {
		case *GeneratedAction:
			if generated.op != op || generated.capabilityKind != kind {
				continue
			}
		case *GeneratedResourceAction:
			if generated.op != op || kind != "resource" {
				continue
			}
		default:
			continue
		}
		capabilities = append(capabilities, core.Capability{
			Type:           core.IntegrationCapabilityTypeAction,
			Name:           action.Name(),
			Label:          action.Label(),
			Description:    action.Description(),
			Configuration:  action.Configuration(),
			OutputChannels: action.OutputChannels(nil),
		})
	}
	return capabilities
}

func capabilityGroupsForActions(actions []core.Action) []core.CapabilityGroup {
	var groups []core.CapabilityGroup
	for _, group := range []struct {
		label string
		op    string
		kind  string
	}{
		{label: "Create Resources", op: "create", kind: "resource"},
		{label: "Read Resources", op: "read", kind: "resource"},
		{label: "Update Resources", op: "update", kind: "resource"},
		{label: "Delete Resources", op: "delete", kind: "resource"},
		{label: "Data Sources", op: "data", kind: "data_source"},
		{label: "Actions", op: "action", kind: "action"},
	} {
		capabilities := capabilitiesForOperation(actions, group.op, group.kind)
		if len(capabilities) > 0 {
			groups = append(groups, core.CapabilityGroup{Label: group.label, Capabilities: capabilities})
		}
	}
	return groups
}

func capabilityGroupsForTriggers(triggers []core.Trigger) []core.CapabilityGroup {
	if len(triggers) == 0 {
		return nil
	}

	capabilities := make([]core.Capability, 0, len(triggers))
	for _, trigger := range triggers {
		capabilities = append(capabilities, core.Capability{
			Type:          core.IntegrationCapabilityTypeTrigger,
			Name:          trigger.Name(),
			Label:         trigger.Label(),
			Description:   trigger.Description(),
			Configuration: trigger.Configuration(),
		})
	}

	return []core.CapabilityGroup{{Label: "Resource Triggers", Capabilities: capabilities}}
}

func TerraformIntegrationName(providerName string) string {
	return "terraform_" + providerName
}

func translateProviderConfig(block Block) ([]configuration.Field, []DroppedCapability, error) {
	var fields []configuration.Field
	var dropped []DroppedCapability
	for name, attr := range block.Attributes {
		single := Block{Attributes: map[string]Attribute{name: attr}}
		translated, ok := TranslateBlock(single)
		if ok {
			fields = append(fields, translated.InputFields...)
			continue
		}
		if attr.Required {
			return nil, nil, fmt.Errorf("unsupported required provider config field %q", name)
		}
		dropped = append(dropped, DroppedCapability{Name: name, Reason: "unsupported optional provider config field"})
	}
	return fields, dropped, nil
}

func isExposed(expose any, name string) bool {
	if expose == nil || expose == "*" {
		return true
	}

	switch values := expose.(type) {
	case []string:
		for _, value := range values {
			if value == name {
				return true
			}
		}
	case []any:
		for _, value := range values {
			if s, ok := value.(string); ok && s == name {
				return true
			}
		}
	}

	return false
}

func terraformResourceName(providerName, terraformName string) string {
	prefix := providerName + "_"
	trimmed := strings.TrimPrefix(terraformName, prefix)
	parts := strings.Split(trimmed, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = upperFirst(parts[i])
	}
	return strings.Join(parts, "")
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
