package terraform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/integrations/terraform"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/testutil"
)

func TestTranslateBlock_RequiredString(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"name": {
				Type:     []byte(`"string"`),
				Required: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeString, result.InputFields[0].Type)
	assert.True(t, result.InputFields[0].Required)
	assert.Empty(t, result.SensitiveAttrs)
}

func TestTranslateBlock_OptionalString(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"name": {
				Type:     []byte(`"string"`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.False(t, result.InputFields[0].Required)
}

func TestTranslateBlock_SensitiveString(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"token": {
				Type:      []byte(`"string"`),
				Optional:  true,
				Sensitive: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.True(t, result.InputFields[0].Sensitive)
	assert.Contains(t, result.SensitiveAttrs, "token")
}

func TestTranslateBlock_WriteOnlyString(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"password": {
				Type:      []byte(`"string"`),
				Optional:  true,
				WriteOnly: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.True(t, result.InputFields[0].Sensitive)
	assert.Contains(t, result.SensitiveAttrs, "password")
}

func TestTranslateBlock_Bool(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"enabled": {
				Type:     []byte(`"bool"`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeBool, result.InputFields[0].Type)
}

func TestTranslateBlock_Number(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"count": {
				Type:     []byte(`"number"`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeNumber, result.InputFields[0].Type)
}

func TestTranslateBlock_ListString(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"tags": {
				Type:     []byte(`["list","string"]`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeList, result.InputFields[0].Type)
	require.NotNil(t, result.InputFields[0].TypeOptions)
	require.NotNil(t, result.InputFields[0].TypeOptions.List)
	assert.Equal(t, configuration.FieldTypeString, result.InputFields[0].TypeOptions.List.ItemDefinition.Type)
}

func TestTranslateBlock_ListBoolRejection(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"flags": {
				Type:     []byte(`["list","bool"]`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	assert.False(t, ok, "block with list(bool) must be rejected")
	assert.Empty(t, result.InputFields)
	assert.Empty(t, result.SensitiveAttrs)
}

func TestTranslateBlock_SetNumber(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"ports": {
				Type:     []byte(`["set","number"]`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeList, result.InputFields[0].Type)
	require.NotNil(t, result.InputFields[0].TypeOptions)
	require.NotNil(t, result.InputFields[0].TypeOptions.List)
	assert.Equal(t, configuration.FieldTypeNumber, result.InputFields[0].TypeOptions.List.ItemDefinition.Type)
}

func TestTranslateBlock_SetBoolRejection(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"flags": {
				Type:     []byte(`["set","bool"]`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	assert.False(t, ok, "block with set(bool) must be rejected")
	assert.Empty(t, result.InputFields)
}

func TestTranslateBlock_MapString(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"labels": {
				Type:     []byte(`["map","string"]`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeObject, result.InputFields[0].Type)
	require.NotNil(t, result.InputFields[0].TypeOptions)
	require.NotNil(t, result.InputFields[0].TypeOptions.Object)
	assert.Empty(t, result.InputFields[0].TypeOptions.Object.Schema)
}

func TestTranslateBlock_Object(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"config": {
				Type:     []byte(`["object",{"host":"string","port":"number"}]`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeObject, result.InputFields[0].Type)
	require.NotNil(t, result.InputFields[0].TypeOptions)
	require.NotNil(t, result.InputFields[0].TypeOptions.Object)
	require.Len(t, result.InputFields[0].TypeOptions.Object.Schema, 2)
}

func TestTranslateBlock_NestedBlockSingle(t *testing.T) {
	block := terraform.Block{
		BlockTypes: map[string]terraform.NestedBlockType{
			"timeouts": {
				NestingMode: "single",
				Block: terraform.Block{
					Attributes: map[string]terraform.Attribute{
						"create": {
							Type:     []byte(`"string"`),
							Optional: true,
						},
					},
				},
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeObject, result.InputFields[0].Type)
	require.NotNil(t, result.InputFields[0].TypeOptions)
	require.NotNil(t, result.InputFields[0].TypeOptions.Object)
	require.Len(t, result.InputFields[0].TypeOptions.Object.Schema, 1)
}

func TestTranslateBlock_NestedBlockList(t *testing.T) {
	block := terraform.Block{
		BlockTypes: map[string]terraform.NestedBlockType{
			"items": {
				NestingMode: "list",
				Block: terraform.Block{
					Attributes: map[string]terraform.Attribute{
						"name": {
							Type:     []byte(`"string"`),
							Optional: true,
						},
					},
				},
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, configuration.FieldTypeList, result.InputFields[0].Type)
	require.NotNil(t, result.InputFields[0].TypeOptions)
	require.NotNil(t, result.InputFields[0].TypeOptions.List)
	assert.Equal(t, configuration.FieldTypeObject, result.InputFields[0].TypeOptions.List.ItemDefinition.Type)
}

func TestTranslateBlock_NestedBlockListRespectsMinAndMaxItems(t *testing.T) {
	block := terraform.Block{
		BlockTypes: map[string]terraform.NestedBlockType{
			"node_pools": {
				NestingMode: "list",
				MinItems:    1,
				MaxItems:    3,
				Block: terraform.Block{
					Attributes: map[string]terraform.Attribute{
						"node_quantity": {
							Type:     []byte(`"number"`),
							Required: true,
						},
					},
				},
			},
		},
	}

	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, "node_pools", result.InputFields[0].Name)
	assert.True(t, result.InputFields[0].Required)
	require.NotNil(t, result.InputFields[0].TypeOptions)
	require.NotNil(t, result.InputFields[0].TypeOptions.List)
	require.NotNil(t, result.InputFields[0].TypeOptions.List.MaxItems)
	assert.Equal(t, 3, *result.InputFields[0].TypeOptions.List.MaxItems)
}

func TestTranslateBlock_NestedBlockMapRejection(t *testing.T) {
	block := terraform.Block{
		BlockTypes: map[string]terraform.NestedBlockType{
			"items": {
				NestingMode: "map",
				Block: terraform.Block{
					Attributes: map[string]terraform.Attribute{
						"name": {
							Type:     []byte(`"string"`),
							Optional: true,
						},
					},
				},
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	assert.False(t, ok, "block with nested block map must be rejected")
	assert.Empty(t, result.InputFields)
}

func TestTranslateBlock_TupleRejection(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"pair": {
				Type:     []byte(`["tuple",["string","number"]]`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	assert.False(t, ok, "block with tuple must be rejected")
	assert.Empty(t, result.InputFields)
}

func TestTranslateBlock_DynamicRejection(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"anything": {
				Type:     []byte(`"dynamic"`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	assert.False(t, ok, "block with dynamic must be rejected")
	assert.Empty(t, result.InputFields)
}

func TestTranslateBlock_MapOfObjectRejection(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"mapping": {
				Type:     []byte(`["map",["object",{"name":"string"}]]`),
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	assert.False(t, ok, "block with map(object) must be rejected")
	assert.Empty(t, result.InputFields)
}

func TestTranslateBlock_SelfRecursiveRejection(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"self": {
				NestedType: &terraform.NestedType{
					NestingMode: "single",
					Attributes: map[string]terraform.Attribute{
						"name": {
							Type:     []byte(`"string"`),
							Optional: true,
						},
						"child": {
							NestedType: &terraform.NestedType{
								NestingMode: "single",
								Attributes: map[string]terraform.Attribute{
									"name": {
										Type:     []byte(`"string"`),
										Optional: true,
									},
									"self": {
										NestedType: &terraform.NestedType{
											NestingMode: "single",
											Attributes:  map[string]terraform.Attribute{},
										},
										Optional: true,
									},
								},
							},
							Optional: true,
						},
					},
				},
				Optional: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	assert.False(t, ok, "block with self-recursive nested type must be rejected")
	assert.Empty(t, result.InputFields)
}

func TestTranslateBlock_DeepNestingSurvives(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-deep-nesting.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/deep"]
	rsc := prov.ResourceSchemas["deep_resource"]
	result, ok := terraform.TranslateBlock(rsc.Block)
	require.True(t, ok)
	require.NotEmpty(t, result.InputFields)
	require.NotEmpty(t, result.SchemaHash)
}

func TestTranslateBlock_ComputedOnlyExcludedFromInput(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"id": {
				Type:     []byte(`"string"`),
				Computed: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	assert.Empty(t, result.InputFields, "computed-only attribute should be excluded from input")
	require.Len(t, result.OutputFields, 1)
	assert.Equal(t, "id", result.OutputFields[0].Name)
}

func TestTranslateBlock_ComputedSensitiveInOutput(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"secret": {
				Type:      []byte(`"string"`),
				Computed:  true,
				Sensitive: true,
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	assert.Empty(t, result.InputFields)
	require.Len(t, result.OutputFields, 1)
	assert.True(t, result.OutputFields[0].Sensitive)
	assert.Contains(t, result.SensitiveAttrs, "secret")
}

func TestTranslateBlock_NestedSensitivePath(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"credentials": {
				NestedType: &terraform.NestedType{
					NestingMode: "single",
					Attributes: map[string]terraform.Attribute{
						"password": {
							Type:      []byte(`"string"`),
							Computed:  true,
							Sensitive: true,
						},
					},
				},
				Computed: true,
			},
		},
	}

	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	assert.Contains(t, result.SensitiveAttrs, "credentials.password")
}

func TestTranslateBlock_DescriptionCarriesThrough(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"name": {
				Type:        []byte(`"string"`),
				Optional:    true,
				Description: "The name of the thing",
			},
		},
	}
	result, ok := terraform.TranslateBlock(block)
	require.True(t, ok)
	require.Len(t, result.InputFields, 1)
	assert.Equal(t, "The name of the thing", result.InputFields[0].Description)
}

func TestTranslateBlock_NestedObjectFromFixture(t *testing.T) {
	raw := testutil.LoadFixture(t, "talos-schema-minimal.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/siderolabs/talos"]
	rsc := prov.ResourceSchemas["talos_machine_configuration_apply"]
	result, ok := terraform.TranslateBlock(rsc.Block)
	require.True(t, ok)

	cc := findField(result.InputFields, "client_configuration")
	require.NotNil(t, cc)
	assert.Equal(t, configuration.FieldTypeObject, cc.Type)
	require.NotNil(t, cc.TypeOptions)
	require.NotNil(t, cc.TypeOptions.Object)
	require.Len(t, cc.TypeOptions.Object.Schema, 3)
}

func TestTranslateBlock_SchemaHashDeterministic(t *testing.T) {
	block := terraform.Block{
		Attributes: map[string]terraform.Attribute{
			"name": {
				Type:     []byte(`"string"`),
				Required: true,
			},
		},
	}
	result1, ok1 := terraform.TranslateBlock(block)
	result2, ok2 := terraform.TranslateBlock(block)
	require.True(t, ok1)
	require.True(t, ok2)
	assert.Equal(t, result1.SchemaHash, result2.SchemaHash)
	assert.NotEmpty(t, result1.SchemaHash)
}

func findField(fields []configuration.Field, name string) *configuration.Field {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}
