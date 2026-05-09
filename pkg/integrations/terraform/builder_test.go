package terraform

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/testutil"
)

func TestBuildIntegration_TalosFixture(t *testing.T) {
	schemas := loadSchemas(t, "talos-schema-minimal.json")
	integration, dropped, err := BuildIntegration(talosConfig(), *schemas, fakeValidator(nil), nil, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "terraform_talos", integration.Name())
	assert.Equal(t, "Talos", integration.Label())
	assert.Equal(t, "terraform", integration.Origin())
	assert.Equal(t, "registry.terraform.io/siderolabs/talos", integration.Source())
	assert.Equal(t, "0.11.0", integration.Version())
	assert.Empty(t, dropped)
	assert.NotNil(t, integration.SetupProvider())

	actionNames := map[string]struct{}{}
	for _, action := range integration.Actions() {
		actionNames[action.Name()] = struct{}{}
	}

	for _, name := range []string{
		"terraform_talos.machineConfigurationApply.create",
		"terraform_talos.machineSecrets.create",
		"terraform_talos.machineConfiguration.data",
		"terraform_talos.clusterKubeconfig.create",
		"terraform_talos.clusterKubeconfig.data",
	} {
		assert.Contains(t, actionNames, name)
	}

	groups := integration.SetupProvider().CapabilityGroups()
	require.Len(t, groups, 6)
	assert.Equal(t, "Create Resources", groups[0].Label)
	assert.Equal(t, "Read Resources", groups[1].Label)
	assert.Equal(t, "Update Resources", groups[2].Label)
	assert.Equal(t, "Delete Resources", groups[3].Label)
	assert.Equal(t, "Data Sources", groups[4].Label)
	assert.Equal(t, "Resource Triggers", groups[5].Label)
	assert.NotEmpty(t, groups[0].Capabilities)
	assert.NotEmpty(t, groups[1].Capabilities)
	for _, capability := range groups[0].Capabilities {
		if capability.Name == "terraform_talos.machineSecrets.create" {
			assert.Equal(t, "Create Machine Secrets", capability.Label)
		}
	}
	for _, capability := range groups[4].Capabilities {
		if capability.Name == "terraform_talos.machineConfiguration.data" {
			assert.Equal(t, "Get Machine Configuration", capability.Label)
		}
	}
	triggerCapability := capabilityByName(groups[5].Capabilities, "terraform_talos.machineSecrets.onChanged")
	require.NotNil(t, triggerCapability)
	assert.Equal(t, core.IntegrationCapabilityTypeTrigger, triggerCapability.Type)
	assert.Equal(t, "On Machine Secrets Changed", triggerCapability.Label)
	assert.Len(t, integration.Configuration(), 1)
}

func TestBuildIntegration_NamespacesTerraformIntegrationName(t *testing.T) {
	schemas := ProviderSchemasFile{ProviderSchemas: map[string]ProviderSchema{
		"registry.terraform.io/cloudflare/cloudflare": {
			Provider: SchemaRepresentation{Block: Block{}},
			ResourceSchemas: map[string]SchemaRepresentation{
				"cloudflare_zone": {
					Block: Block{Attributes: map[string]Attribute{
						"id":   {Type: rawJSON(t, `"string"`), Computed: true},
						"name": {Type: rawJSON(t, `"string"`), Optional: true},
					}},
				},
			},
		},
	}}

	integration, _, err := BuildIntegration(config.TerraformProviderIntegration{
		Name:    "cloudflare",
		Label:   "Cloudflare",
		Icon:    "terraform",
		Source:  "registry.terraform.io/cloudflare/cloudflare",
		Version: "5.19.1",
		Expose:  config.TerraformProviderExpose{Resources: []string{"cloudflare_zone"}, DataSources: []string{}},
	}, schemas, fakeValidator(nil), nil, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "terraform_cloudflare", integration.Name())
	assert.Equal(t, "Cloudflare", integration.Label())
	require.NotEmpty(t, integration.Actions())
	action := integration.Actions()[0]
	assert.Equal(t, "terraform_cloudflare.zone.create", action.Name())
	assert.Equal(t, "terraform", action.Icon())
	generated, ok := action.(*GeneratedResourceAction)
	require.True(t, ok)
	assert.Equal(t, "cloudflare", generated.base.ProviderName())

	require.NotEmpty(t, integration.Triggers())
	assert.Equal(t, "terraform_cloudflare.zone.onChanged", integration.Triggers()[0].Name())
	assert.Equal(t, "terraform", integration.Triggers()[0].Icon())
}

func TestBuildIntegration_DropsUnsupportedResource(t *testing.T) {
	schemas := loadSchemas(t, "synthetic-dynamic-type.json")
	integration, dropped, err := BuildIntegration(config.TerraformProviderIntegration{
		Name:    "dynamic",
		Label:   "Synthetic",
		Source:  "registry.terraform.io/example/dynamic",
		Version: "1.0.0",
		Expose:  config.TerraformProviderExpose{Resources: "*", DataSources: "*"},
	}, *schemas, fakeValidator(nil), nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, integration.Actions())
	require.Len(t, dropped, 1)
	assert.Contains(t, dropped[0].Reason, "unsupported")
}

func TestBuildIntegrationInjectsRunner(t *testing.T) {
	schemas := loadSchemas(t, "talos-schema-minimal.json")
	runner := &testActionRunner{}
	integration, _, err := BuildIntegration(talosConfig(), *schemas, fakeValidator(nil), runner, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, integration.Actions())
	var generated *GeneratedAction
	for _, action := range integration.Actions() {
		if candidate, ok := action.(*GeneratedAction); ok {
			generated = candidate
			break
		}
	}
	require.NotNil(t, generated)
	assert.Same(t, runner, generated.runner)
}

type testActionRunner struct{}

func (r *testActionRunner) Execute(ctx core.ExecutionContext, action *GeneratedAction) error {
	return nil
}

func TestBuildIntegration_ExposeFilters(t *testing.T) {
	schemas := loadSchemas(t, "talos-schema-minimal.json")
	cfg := talosConfig()
	cfg.Expose.Resources = []string{"talos_machine_secrets"}
	cfg.Expose.DataSources = []string{"talos_machine_configuration"}

	integration, _, err := BuildIntegration(cfg, *schemas, fakeValidator(nil), nil, nil, nil)
	require.NoError(t, err)

	var names []string
	for _, action := range integration.Actions() {
		names = append(names, action.Name())
	}
	assert.ElementsMatch(t, []string{
		"terraform_talos.machineSecrets.create",
		"terraform_talos.machineSecrets.read",
		"terraform_talos.machineSecrets.update",
		"terraform_talos.machineSecrets.delete",
		"terraform_talos.machineConfiguration.data",
	}, names)

	var triggerNames []string
	for _, trigger := range integration.Triggers() {
		triggerNames = append(triggerNames, trigger.Name())
	}
	assert.ElementsMatch(t, []string{"terraform_talos.machineSecrets.onChanged"}, triggerNames)
}

func TestBuildIntegration_UnsupportedRequiredProviderConfigFails(t *testing.T) {
	schemas := ProviderSchemasFile{ProviderSchemas: map[string]ProviderSchema{
		"registry.terraform.io/example/provider": {
			Provider: SchemaRepresentation{Block: Block{Attributes: map[string]Attribute{
				"bad": {Type: rawJSON(t, `"dynamic"`), Required: true},
			}}},
		},
	}}

	_, _, err := BuildIntegration(config.TerraformProviderIntegration{
		Name:    "provider",
		Source:  "registry.terraform.io/example/provider",
		Version: "1.0.0",
		Expose:  config.TerraformProviderExpose{Resources: "*", DataSources: "*"},
	}, schemas, fakeValidator(nil), nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported required provider config field")
}

func TestBuildIntegration_UnsupportedOptionalProviderConfigDropped(t *testing.T) {
	schemas := ProviderSchemasFile{ProviderSchemas: map[string]ProviderSchema{
		"registry.terraform.io/example/provider": {
			Provider: SchemaRepresentation{Block: Block{Attributes: map[string]Attribute{
				"bad":  {Type: rawJSON(t, `"dynamic"`), Optional: true},
				"good": {Type: rawJSON(t, `"string"`), Optional: true},
			}}},
		},
	}}

	integration, dropped, err := BuildIntegration(config.TerraformProviderIntegration{
		Name:    "provider",
		Source:  "registry.terraform.io/example/provider",
		Version: "1.0.0",
		Expose:  config.TerraformProviderExpose{Resources: "*", DataSources: "*"},
	}, schemas, fakeValidator(nil), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, dropped, 1)
	assert.Equal(t, "bad", dropped[0].Name)
	assert.Equal(t, []configuration.Field{{Name: "good", Label: "good", Type: configuration.FieldTypeString}}, integration.Configuration())
}

func loadSchemas(t *testing.T, fixture string) *ProviderSchemasFile {
	t.Helper()
	schemas, err := ParseProviderSchemas(testutil.LoadFixture(t, fixture))
	require.NoError(t, err)
	return schemas
}

func talosConfig() config.TerraformProviderIntegration {
	return config.TerraformProviderIntegration{
		Name:        "talos",
		Label:       "Talos",
		Description: "Terraform provider registry.terraform.io/siderolabs/talos",
		Source:      "registry.terraform.io/siderolabs/talos",
		Version:     "0.11.0",
		Expose:      config.TerraformProviderExpose{Resources: "*", DataSources: "*"},
	}
}

func rawJSON(t *testing.T, s string) json.RawMessage {
	t.Helper()
	return json.RawMessage(s)
}
