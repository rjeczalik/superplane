package v6

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONToDynamicValueAndBack(t *testing.T) {
	tests := []struct {
		name   string
		schema tftypes.Type
		raw    string
		want   string
	}{
		{name: "string", schema: tftypes.String, raw: `"hello"`, want: `"hello"`},
		{name: "number", schema: tftypes.Number, raw: `42.5`, want: `42.5`},
		{name: "bool", schema: tftypes.Bool, raw: `true`, want: `true`},
		{
			name: "object",
			schema: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
				"name":    tftypes.String,
				"enabled": tftypes.Bool,
				"count":   tftypes.Number,
			}},
			raw:  `{"name":"resource","enabled":true,"count":3}`,
			want: `{"name":"resource","enabled":true,"count":3}`,
		},
		{name: "list", schema: tftypes.List{ElementType: tftypes.String}, raw: `["a","b"]`, want: `["a","b"]`},
		{name: "set", schema: tftypes.Set{ElementType: tftypes.String}, raw: `["a","b"]`, want: `["a","b"]`},
		{name: "null", schema: tftypes.String, raw: `null`, want: `null`},
		{
			name: "nested object",
			schema: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
				"name": tftypes.String,
				"tags": tftypes.Map{ElementType: tftypes.String},
				"ports": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
					"name": tftypes.String,
					"port": tftypes.Number,
				}}},
			}},
			raw:  `{"name":"svc","tags":{"env":"dev"},"ports":[{"name":"http","port":80}]}`,
			want: `{"name":"svc","tags":{"env":"dev"},"ports":[{"name":"http","port":80}]}`,
		},
		{
			name:   "dynamic pseudo type",
			schema: tftypes.DynamicPseudoType,
			raw:    `{"type":"string","value":"selected"}`,
			want:   `{"type":"string","value":"selected"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dynamic, err := JSONToDynamicValue(tt.schema, []byte(tt.raw))
			require.NoError(t, err)
			require.NotNil(t, dynamic)
			require.NotEmpty(t, dynamic.MsgPack)

			got, err := DynamicValueToJSON(tt.schema, dynamic)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

func TestDynamicValueToJSON_UnknownSentinel(t *testing.T) {
	schema := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String,
	}}

	dynamic, err := JSONToDynamicValue(schema, []byte(`{"$terraformUnknown":true,"type":["object",{"id":"string"}]}`))
	require.NoError(t, err)

	got, err := DynamicValueToJSON(schema, dynamic)
	require.NoError(t, err)
	assert.JSONEq(t, `{"$terraformUnknown":true,"type":["object",{"id":"string"}]}`, string(got))
}

func TestDynamicValueToJSON_ComputedNullNestedValue(t *testing.T) {
	schema := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":   tftypes.String,
		"name": tftypes.String,
	}}
	value := tftypes.NewValue(schema, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, nil),
		"name": tftypes.NewValue(tftypes.String, "known"),
	})

	dynamic, err := tfprotov6.NewDynamicValue(schema, value)
	require.NoError(t, err)

	got, err := DynamicValueToJSON(schema, &dynamic)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":null,"name":"known"}`, string(got))
}

func TestDynamicValueToJSON_PreservesNumberPrecision(t *testing.T) {
	n, _, err := big.ParseFloat("12345678901234567890.125", 10, 512, big.ToNearestEven)
	require.NoError(t, err)
	value := tftypes.NewValue(tftypes.Number, n)

	dynamic, err := tfprotov6.NewDynamicValue(tftypes.Number, value)
	require.NoError(t, err)

	got, err := DynamicValueToJSON(tftypes.Number, &dynamic)
	require.NoError(t, err)

	var number json.Number
	require.NoError(t, json.Unmarshal(got, &number))
	assert.Equal(t, "12345678901234567890.125", number.String())
}

func TestJSONToDynamicValue_NestedUnknownSentinel(t *testing.T) {
	schema := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":     tftypes.String,
		"name":   tftypes.String,
		"config": tftypes.String,
	}}

	raw := `{"id":{"$terraformUnknown":true,"type":"string"},"name":"my-resource","config":{"$terraformUnknown":true,"type":"string"}}`
	dynamic, err := JSONToDynamicValue(schema, []byte(raw))
	require.NoError(t, err)
	require.NotNil(t, dynamic)

	got, err := DynamicValueToJSON(schema, dynamic)
	require.NoError(t, err)
	assert.JSONEq(t, raw, string(got))
}

func TestJSONToDynamicValue_NestedUnknownInList(t *testing.T) {
	schema := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"name": tftypes.String,
		"items": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"id":    tftypes.String,
			"value": tftypes.String,
		}}},
	}}

	raw := `{"name":"test","items":[{"id":{"$terraformUnknown":true,"type":"string"},"value":"known"}]}`
	dynamic, err := JSONToDynamicValue(schema, []byte(raw))
	require.NoError(t, err)

	got, err := DynamicValueToJSON(schema, dynamic)
	require.NoError(t, err)
	assert.JSONEq(t, raw, string(got))
}

func TestJSONToDynamicValue_NullObject(t *testing.T) {
	schema := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String,
	}}

	dynamic, err := JSONToDynamicValue(schema, []byte(`null`))
	require.NoError(t, err)

	got, err := DynamicValueToJSON(schema, dynamic)
	require.NoError(t, err)
	assert.JSONEq(t, `null`, string(got))
}

func TestJSONToDynamicValue_RejectsSchemaMismatch(t *testing.T) {
	_, err := JSONToDynamicValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"name": tftypes.String,
	}}, []byte(`{"name":"ok","extra":"rejected"}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode terraform dynamic JSON")
}

func TestDynamicValueToJSON_RejectsNilDynamicValue(t *testing.T) {
	_, err := DynamicValueToJSON(tftypes.String, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dynamic value is nil")
}
