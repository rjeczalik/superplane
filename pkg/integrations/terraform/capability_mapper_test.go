package terraform

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestCapabilityMapperMapMixedSchemas(t *testing.T) {
	schema := &runtime.CapabilitySchema{
		Resources: map[string]runtime.ResourceSchema{
			"example_server": {Version: 1, Attributes: blockJSON(t, Block{Attributes: map[string]Attribute{"name": {Type: rawJSON(t, `"string"`), Required: true}}})},
		},
		DataSources: map[string]runtime.DataSourceSchema{
			"example_image": {Attributes: blockJSON(t, Block{Attributes: map[string]Attribute{"name": {Type: rawJSON(t, `"string"`), Required: true}}})},
		},
		Actions: map[string]runtime.ActionSchema{
			"example_restart": {
				Attributes:    blockJSON(t, Block{Attributes: map[string]Attribute{"name": {Type: rawJSON(t, `"string"`), Required: true}}}),
				HasPlanStep:   true,
				StreamsEvents: true,
			},
		},
	}

	actions, triggers, capabilities, dropped, err := (&CapabilityMapper{}).Map(schema, mapperConfig(), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, dropped)
	assert.Len(t, actions, 6)
	assert.Len(t, triggers, 1)
	assert.Len(t, capabilities, 7)

	found := map[string]bool{}
	for _, action := range actions {
		found[action.Name()] = true
	}
	assert.True(t, found["terraform_example.server.create"])
	assert.True(t, found["terraform_example.server.read"])
	assert.True(t, found["terraform_example.server.update"])
	assert.True(t, found["terraform_example.server.delete"])
	assert.True(t, found["terraform_example.image.data"])
	assert.True(t, found["terraform_example.restart.action"])

	triggerCapability := capabilityByName(capabilities, "terraform_example.server.onChanged")
	require.NotNil(t, triggerCapability)
	assert.Equal(t, core.IntegrationCapabilityTypeTrigger, triggerCapability.Type)
	assert.Equal(t, "On Server Changed", triggerCapability.Label)
}

func TestCapabilityMapperDropsUnsupportedActionSchema(t *testing.T) {
	schema := &runtime.CapabilitySchema{Actions: map[string]runtime.ActionSchema{
		"example_bad": {Attributes: blockJSON(t, Block{Attributes: map[string]Attribute{"bad": {Type: rawJSON(t, `"dynamic"`), Required: true}}})},
	}}

	actions, _, _, dropped, err := (&CapabilityMapper{}).Map(schema, mapperConfig(), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, actions)
	require.Len(t, dropped, 1)
	assert.Equal(t, "example_bad", dropped[0].Name)
	assert.Contains(t, dropped[0].Reason, "unsupported action")
}

func TestCapabilityMapperExposeFiltersActions(t *testing.T) {
	cfg := mapperConfig()
	cfg.Expose.Actions = []string{"example_keep"}
	schema := &runtime.CapabilitySchema{Actions: map[string]runtime.ActionSchema{
		"example_keep": {Attributes: blockJSON(t, Block{Attributes: map[string]Attribute{"name": {Type: rawJSON(t, `"string"`), Optional: true}}})},
		"example_skip": {Attributes: blockJSON(t, Block{Attributes: map[string]Attribute{"name": {Type: rawJSON(t, `"string"`), Optional: true}}})},
	}}

	actions, _, _, _, err := (&CapabilityMapper{}).Map(schema, cfg, nil, nil)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	assert.Equal(t, "terraform_example.keep.action", actions[0].Name())
}

func mapperConfig() config.TerraformProviderIntegration {
	return config.TerraformProviderIntegration{
		Name:    "example",
		Source:  "registry.terraform.io/example/example",
		Version: "1.0.0",
		Expose:  config.TerraformProviderExpose{Resources: "*", DataSources: "*", Actions: "*"},
	}
}

func blockJSON(t *testing.T, block Block) []byte {
	t.Helper()
	raw, err := json.Marshal(block)
	require.NoError(t, err)
	return raw
}

func capabilityByName(capabilities []core.Capability, name string) *core.Capability {
	for _, capability := range capabilities {
		if capability.Name == name {
			return &capability
		}
	}
	return nil
}

func assertAction(t *testing.T, action *GeneratedAction) {
	t.Helper()
	assert.Equal(t, "example", action.providerName)
	assert.Equal(t, "registry.terraform.io/example/example", action.providerSource)
	assert.Equal(t, "1.0.0", action.providerVersion)
}
