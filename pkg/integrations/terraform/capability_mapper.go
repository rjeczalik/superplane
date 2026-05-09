package terraform

import (
	"encoding/json"
	"fmt"

	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

type CapabilityMapper struct{}

func (m *CapabilityMapper) Map(schema *runtime.CapabilitySchema, cfg config.TerraformProviderIntegration, runner ActionRunner, resourceRunner *ResourceRunner) ([]core.Action, []core.Trigger, []core.Capability, []DroppedCapability, error) {
	if schema == nil {
		return nil, nil, nil, nil, fmt.Errorf("terraform capability schema is required")
	}

	var actions []core.Action
	var triggers []core.Trigger
	var capabilities []core.Capability
	var dropped []DroppedCapability

	for name, schema := range schema.Resources {
		if !isExposed(cfg.Expose.Resources, name) {
			continue
		}
		base, _, ok := buildAction(cfg, name, "create", schemaRepresentationFromRuntime(schema.Version, schema.Attributes), nil)
		if !ok {
			dropped = append(dropped, DroppedCapability{Name: name, Reason: "unsupported resource schema"})
			continue
		}
		base.capabilityKind = "resource"
		for _, op := range []string{"create", "read", "update", "delete"} {
			action := NewGeneratedResourceAction(base, op, resourceRunner)
			actions = append(actions, action)
			capabilities = append(capabilities, core.Capability{
				Type:           core.IntegrationCapabilityTypeAction,
				Name:           action.Name(),
				Label:          action.Label(),
				Description:    action.Description(),
				Configuration:  action.Configuration(),
				OutputChannels: action.OutputChannels(nil),
			})
		}
		trigger := NewGeneratedResourceTrigger(TerraformIntegrationName(cfg.Name), base.resourceName, name, cfg.Icon)
		triggers = append(triggers, trigger)
		capabilities = append(capabilities, core.Capability{
			Type:          core.IntegrationCapabilityTypeTrigger,
			Name:          trigger.Name(),
			Label:         trigger.Label(),
			Description:   trigger.Description(),
			Configuration: trigger.Configuration(),
		})
	}

	for name, schema := range schema.DataSources {
		if !isExposed(cfg.Expose.DataSources, name) {
			continue
		}
		action, capability, ok := buildAction(cfg, name, "data", schemaRepresentationFromRuntime(0, schema.Attributes), runner)
		if !ok {
			dropped = append(dropped, DroppedCapability{Name: name, Reason: "unsupported data source schema"})
			continue
		}
		action.capabilityKind = "data_source"
		actions = append(actions, action)
		capabilities = append(capabilities, capability)
	}

	for name, schema := range schema.Actions {
		if !isExposed(cfg.Expose.Actions, name) {
			continue
		}
		action, capability, ok := buildAction(cfg, name, "action", schemaRepresentationFromRuntime(0, schema.Attributes), runner)
		if !ok {
			dropped = append(dropped, DroppedCapability{Name: name, Reason: "unsupported action schema"})
			continue
		}
		action.hasPlanStep = schema.HasPlanStep
		action.streamsEvents = schema.StreamsEvents
		action.capabilityKind = "action"
		actions = append(actions, action)
		capabilities = append(capabilities, capability)
	}

	return actions, triggers, capabilities, dropped, nil
}

func schemaRepresentationFromRuntime(version int64, raw []byte) SchemaRepresentation {
	var block Block
	_ = json.Unmarshal(raw, &block)
	return SchemaRepresentation{Version: version, Block: block}
}

func capabilitySchemaFromProvider(provider ProviderSchema) *runtime.CapabilitySchema {
	out := &runtime.CapabilitySchema{
		Resources:   make(map[string]runtime.ResourceSchema, len(provider.ResourceSchemas)),
		DataSources: make(map[string]runtime.DataSourceSchema, len(provider.DataSourceSchemas)),
		Actions:     make(map[string]runtime.ActionSchema),
	}
	for name, schema := range provider.ResourceSchemas {
		out.Resources[name] = runtime.ResourceSchema{Version: schema.Version, Attributes: mustMarshalBlock(schema.Block)}
	}
	for name, schema := range provider.DataSourceSchemas {
		out.DataSources[name] = runtime.DataSourceSchema{Attributes: mustMarshalBlock(schema.Block)}
	}
	return out
}

func mustMarshalBlock(block Block) []byte {
	raw, _ := json.Marshal(block)
	return raw
}
