package terraform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superplanehq/superplane/pkg/integrations/terraform"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/testutil"
)

func TestParseProviderSchemas_TalosMinimal(t *testing.T) {
	raw := testutil.LoadFixture(t, "talos-schema-minimal.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)
	require.NotNil(t, schemas)
	assert.Equal(t, "1.0", schemas.FormatVersion)
	require.Len(t, schemas.ProviderSchemas, 1)

	talos := schemas.ProviderSchemas["registry.terraform.io/siderolabs/talos"]
	require.NotNil(t, talos)

	assert.Contains(t, talos.ResourceSchemas, "talos_machine_secrets")
	assert.Contains(t, talos.ResourceSchemas, "talos_machine_configuration_apply")
	assert.Contains(t, talos.ResourceSchemas, "talos_machine_bootstrap")
	assert.Contains(t, talos.ResourceSchemas, "talos_cluster_kubeconfig")

	assert.Contains(t, talos.DataSourceSchemas, "talos_client_configuration")
	assert.Contains(t, talos.DataSourceSchemas, "talos_machine_configuration")
	assert.Contains(t, talos.DataSourceSchemas, "talos_cluster_health")
	assert.Contains(t, talos.DataSourceSchemas, "talos_cluster_kubeconfig")

	rsc := talos.ResourceSchemas["talos_machine_configuration_apply"]
	assert.True(t, hasAttribute(rsc.Block, "node"))
	assert.True(t, hasAttribute(rsc.Block, "client_configuration"))
	assert.True(t, hasNestedBlock(rsc.Block, "timeouts"))
}

func TestParseProviderSchemas_DeepNesting(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-deep-nesting.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/deep"]
	rsc := prov.ResourceSchemas["deep_resource"]

	top, ok := rsc.Block.Attributes["top"]
	require.True(t, ok)
	require.NotNil(t, top.Type)

	tp, err := terraform.ParseType(top.Type)
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindObject, tp.Kind)
	assert.Contains(t, tp.Attributes, "middle")
}

func TestParseProviderSchemas_DynamicType(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-dynamic-type.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/dynamic"]
	rsc := prov.ResourceSchemas["dynamic_resource"]

	attr, ok := rsc.Block.Attributes["anything"]
	require.True(t, ok)

	tp, err := terraform.ParseType(attr.Type)
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindDynamic, tp.Kind)
}

func TestParseProviderSchemas_Tuple(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-tuple.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/tuple"]
	rsc := prov.ResourceSchemas["tuple_resource"]

	attr, ok := rsc.Block.Attributes["pair"]
	require.True(t, ok)

	tp, err := terraform.ParseType(attr.Type)
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindTuple, tp.Kind)
	require.Len(t, tp.TupleElems, 2)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.TupleElems[0].Kind)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.TupleElems[1].Kind)
}

func TestParseProviderSchemas_ListBool(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-list-bool.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/listbool"]
	rsc := prov.ResourceSchemas["listbool_resource"]

	attr, ok := rsc.Block.Attributes["flags"]
	require.True(t, ok)

	tp, err := terraform.ParseType(attr.Type)
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindList, tp.Kind)
	require.NotNil(t, tp.Element)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.Element.Kind)
	assert.Equal(t, "bool", tp.Element.Primitive)
}

func TestParseProviderSchemas_SetBool(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-set-bool.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/setbool"]
	rsc := prov.ResourceSchemas["setbool_resource"]

	attr, ok := rsc.Block.Attributes["flags"]
	require.True(t, ok)

	tp, err := terraform.ParseType(attr.Type)
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindSet, tp.Kind)
	require.NotNil(t, tp.Element)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.Element.Kind)
	assert.Equal(t, "bool", tp.Element.Primitive)
}

func TestParseProviderSchemas_NestedBlockMap(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-nested-block-map.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/nestedblockmap"]
	rsc := prov.ResourceSchemas["nestedblockmap_resource"]

	bt, ok := rsc.Block.BlockTypes["items"]
	require.True(t, ok)
	assert.Equal(t, "map", bt.NestingMode)
}

func TestParseProviderSchemas_MapOfObject(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-map-of-object.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/mapofobject"]
	rsc := prov.ResourceSchemas["mapofobject_resource"]

	attr, ok := rsc.Block.Attributes["mapping"]
	require.True(t, ok)

	tp, err := terraform.ParseType(attr.Type)
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindMap, tp.Kind)
	require.NotNil(t, tp.Element)
	assert.Equal(t, terraform.TypeKindObject, tp.Element.Kind)
}

func TestParseProviderSchemas_SelfRecursive(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-self-recursive.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/selfrecursive"]
	rsc := prov.ResourceSchemas["selfrecursive_resource"]

	attr, ok := rsc.Block.Attributes["self"]
	require.True(t, ok)
	require.NotNil(t, attr.NestedType)
	assert.Equal(t, "single", attr.NestedType.NestingMode)
	assert.Contains(t, attr.NestedType.Attributes, "child")
}

func TestParseProviderSchemas_DataOnly(t *testing.T) {
	raw := testutil.LoadFixture(t, "synthetic-data-only.json")
	schemas, err := terraform.ParseProviderSchemas(raw)
	require.NoError(t, err)

	prov := schemas.ProviderSchemas["registry.terraform.io/example/dataonly"]
	assert.Empty(t, prov.ResourceSchemas)
	require.Len(t, prov.DataSourceSchemas, 1)
	assert.Contains(t, prov.DataSourceSchemas, "dataonly_thing")
}

func TestParseType_Primitives(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{`"string"`, "string"},
		{`"bool"`, "bool"},
		{`"number"`, "number"},
	} {
		tp, err := terraform.ParseType([]byte(tc.raw))
		require.NoError(t, err)
		assert.Equal(t, terraform.TypeKindPrimitive, tp.Kind)
		assert.Equal(t, tc.want, tp.Primitive)
	}
}

func TestParseType_Dynamic(t *testing.T) {
	tp, err := terraform.ParseType([]byte(`"dynamic"`))
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindDynamic, tp.Kind)
}

func TestParseType_UnknownPrimitive(t *testing.T) {
	_, err := terraform.ParseType([]byte(`"unknown"`))
	require.Error(t, err)
}

func TestParseType_ListString(t *testing.T) {
	tp, err := terraform.ParseType([]byte(`["list","string"]`))
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindList, tp.Kind)
	require.NotNil(t, tp.Element)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.Element.Kind)
	assert.Equal(t, "string", tp.Element.Primitive)
}

func TestParseType_Object(t *testing.T) {
	tp, err := terraform.ParseType([]byte(`["object",{"a":"string","b":"number"}]`))
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindObject, tp.Kind)
	require.Len(t, tp.Attributes, 2)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.Attributes["a"].Kind)
	assert.Equal(t, "string", tp.Attributes["a"].Primitive)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.Attributes["b"].Kind)
	assert.Equal(t, "number", tp.Attributes["b"].Primitive)
}

func TestParseType_MapString(t *testing.T) {
	tp, err := terraform.ParseType([]byte(`["map","string"]`))
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindMap, tp.Kind)
	require.NotNil(t, tp.Element)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.Element.Kind)
	assert.Equal(t, "string", tp.Element.Primitive)
}

func TestParseType_SetNumber(t *testing.T) {
	tp, err := terraform.ParseType([]byte(`["set","number"]`))
	require.NoError(t, err)
	assert.Equal(t, terraform.TypeKindSet, tp.Kind)
	require.NotNil(t, tp.Element)
	assert.Equal(t, terraform.TypeKindPrimitive, tp.Element.Kind)
	assert.Equal(t, "number", tp.Element.Primitive)
}

func TestParseType_Invalid(t *testing.T) {
	_, err := terraform.ParseType([]byte(`{`))
	require.Error(t, err)
}

func hasAttribute(b terraform.Block, name string) bool {
	_, ok := b.Attributes[name]
	return ok
}

func hasNestedBlock(b terraform.Block, name string) bool {
	_, ok := b.BlockTypes[name]
	return ok
}
