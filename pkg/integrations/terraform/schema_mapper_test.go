package terraform_test

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/integrations/terraform"
)

func TestMapV5SchemaProducesCLICompatibleBlock(t *testing.T) {
	schema := &tfprotov5.Schema{Version: 3, Block: &tfprotov5.SchemaBlock{
		Description: "resource schema",
		Attributes: []*tfprotov5.SchemaAttribute{
			{Name: "name", Type: tftypes.String, Required: true},
			{Name: "id", Type: tftypes.String, Computed: true, Sensitive: true},
		},
		BlockTypes: []*tfprotov5.SchemaNestedBlock{{
			TypeName: "rule",
			Nesting:  tfprotov5.SchemaNestedBlockNestingModeList,
			Block: &tfprotov5.SchemaBlock{Attributes: []*tfprotov5.SchemaAttribute{
				{Name: "port", Type: tftypes.Number, Optional: true},
			}},
		}},
	}}

	mapped, err := terraform.MapV5Schema(schema)
	require.NoError(t, err)
	assert.Equal(t, int64(3), mapped.Version)
	assert.Equal(t, "resource schema", mapped.Block.Description)
	assert.JSONEq(t, `"string"`, string(mapped.Block.Attributes["name"].Type))
	assert.True(t, mapped.Block.Attributes["id"].Sensitive)
	assert.Equal(t, "list", mapped.Block.BlockTypes["rule"].NestingMode)

	translated, ok := terraform.TranslateBlock(mapped.Block)
	require.True(t, ok)
	require.Len(t, translated.InputFields, 2)
	assert.Contains(t, translated.SensitiveAttrs, "id")
}

func TestMapV6SchemaPreservesNestedAttributes(t *testing.T) {
	schema := &tfprotov6.Schema{Block: &tfprotov6.SchemaBlock{Attributes: []*tfprotov6.SchemaAttribute{{
		Name: "config",
		NestedType: &tfprotov6.SchemaObject{
			Nesting: tfprotov6.SchemaObjectNestingModeSingle,
			Attributes: []*tfprotov6.SchemaAttribute{
				{Name: "token", Type: tftypes.String, Required: true, Sensitive: true},
			},
		},
		Optional: true,
	}}}}

	mapped, err := terraform.MapV6Schema(schema)
	require.NoError(t, err)
	attr := mapped.Block.Attributes["config"]
	require.NotNil(t, attr.NestedType)
	assert.Equal(t, "single", attr.NestedType.NestingMode)
	assert.True(t, attr.NestedType.Attributes["token"].Sensitive)

	translated, ok := terraform.TranslateBlock(mapped.Block)
	require.True(t, ok)
	require.Len(t, translated.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeObject, translated.InputFields[0].Type)
	assert.Contains(t, translated.SensitiveAttrs, "config.token")
}

func TestMappedSchemaCanRoundTripThroughJSONParser(t *testing.T) {
	mapped, err := terraform.MapV5Schema(&tfprotov5.Schema{Block: &tfprotov5.SchemaBlock{Attributes: []*tfprotov5.SchemaAttribute{
		{Name: "labels", Type: tftypes.Map{ElementType: tftypes.String}, Optional: true},
	}}})
	require.NoError(t, err)

	raw, err := json.Marshal(terraform.ProviderSchemasFile{
		FormatVersion: "1.0",
		ProviderSchemas: map[string]terraform.ProviderSchema{
			"registry.terraform.io/example/test": {
				ResourceSchemas: map[string]terraform.SchemaRepresentation{"example_test": mapped},
			},
		},
	})
	require.NoError(t, err)

	parsed, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)
	block := parsed.ProviderSchemas["registry.terraform.io/example/test"].ResourceSchemas["example_test"].Block
	_, ok := terraform.TranslateBlock(block)
	assert.True(t, ok)
}
