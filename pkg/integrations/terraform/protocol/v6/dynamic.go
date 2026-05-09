package v6

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const unknownSentinelKey = "$terraformUnknown"

// JSONToDynamicValue converts canonical JSON into a typed Terraform protocol
// DynamicValue. It deliberately routes through tftypes so protocol encoding,
// nulls, dynamic pseudo-types, and collection semantics match Terraform's
// plugin protocol implementation.
func JSONToDynamicValue(schema tftypes.Type, raw []byte) (*tfprotov6.DynamicValue, error) {
	value, err := valueFromCanonicalJSON(schema, raw)
	if err != nil {
		return nil, fmt.Errorf("decode terraform dynamic JSON: %w", err)
	}

	dynamic, err := tfprotov6.NewDynamicValue(schema, value)
	if err != nil {
		return nil, fmt.Errorf("encode terraform dynamic value: %w", err)
	}

	return &dynamic, nil
}

// DynamicValueToJSON converts a typed Terraform protocol DynamicValue into
// canonical JSON for SuperPlane state and action payloads.
func DynamicValueToJSON(schema tftypes.Type, value *tfprotov6.DynamicValue) ([]byte, error) {
	if value == nil {
		return nil, fmt.Errorf("dynamic value is nil")
	}

	decoded, err := value.Unmarshal(schema)
	if err != nil {
		return nil, fmt.Errorf("decode terraform dynamic value: %w", err)
	}

	out, err := canonicalJSONFromValue(decoded, schema)
	if err != nil {
		return nil, fmt.Errorf("encode terraform dynamic JSON: %w", err)
	}

	return json.Marshal(out)
}

func valueFromCanonicalJSON(schema tftypes.Type, raw []byte) (tftypes.Value, error) {
	if isUnknownSentinel(raw) {
		return tftypes.NewValue(schema, tftypes.UnknownValue), nil
	}

	switch typ := schema.(type) {
	case tftypes.Object:
		var attrs map[string]json.RawMessage
		if err := json.Unmarshal(raw, &attrs); err != nil {
			return tftypes.Value{}, fmt.Errorf("decode object: %w", err)
		}
		if attrs == nil {
			return tftypes.NewValue(schema, nil), nil
		}
		for key := range attrs {
			if _, ok := typ.AttributeTypes[key]; !ok {
				return tftypes.Value{}, fmt.Errorf("unsupported attribute %q", key)
			}
		}
		values := make(map[string]tftypes.Value, len(typ.AttributeTypes))
		for name, attrType := range typ.AttributeTypes {
			attrRaw, ok := attrs[name]
			if !ok {
				values[name] = tftypes.NewValue(attrType, nil)
				continue
			}
			val, err := valueFromCanonicalJSON(attrType, attrRaw)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("AttributeName(%q): %w", name, err)
			}
			values[name] = val
		}
		return tftypes.NewValue(schema, values), nil

	case tftypes.List:
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return tftypes.Value{}, fmt.Errorf("decode list: %w", err)
		}
		if elements == nil {
			return tftypes.NewValue(schema, nil), nil
		}
		values := make([]tftypes.Value, len(elements))
		for i, elemRaw := range elements {
			val, err := valueFromCanonicalJSON(typ.ElementType, elemRaw)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("ElementKeyInt(%d): %w", i, err)
			}
			values[i] = val
		}
		return tftypes.NewValue(schema, values), nil

	case tftypes.Set:
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return tftypes.Value{}, fmt.Errorf("decode set: %w", err)
		}
		if elements == nil {
			return tftypes.NewValue(schema, nil), nil
		}
		values := make([]tftypes.Value, len(elements))
		for i, elemRaw := range elements {
			val, err := valueFromCanonicalJSON(typ.ElementType, elemRaw)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("ElementKeyInt(%d): %w", i, err)
			}
			values[i] = val
		}
		return tftypes.NewValue(schema, values), nil

	case tftypes.Map:
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return tftypes.Value{}, fmt.Errorf("decode map: %w", err)
		}
		if entries == nil {
			return tftypes.NewValue(schema, nil), nil
		}
		values := make(map[string]tftypes.Value, len(entries))
		for key, entryRaw := range entries {
			val, err := valueFromCanonicalJSON(typ.ElementType, entryRaw)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("ElementKeyString(%q): %w", key, err)
			}
			values[key] = val
		}
		return tftypes.NewValue(schema, values), nil

	case tftypes.Tuple:
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return tftypes.Value{}, fmt.Errorf("decode tuple: %w", err)
		}
		if elements == nil {
			return tftypes.NewValue(schema, nil), nil
		}
		if len(elements) != len(typ.ElementTypes) {
			return tftypes.Value{}, fmt.Errorf("tuple length %d does not match schema length %d", len(elements), len(typ.ElementTypes))
		}
		values := make([]tftypes.Value, len(elements))
		for i, elemRaw := range elements {
			val, err := valueFromCanonicalJSON(typ.ElementTypes[i], elemRaw)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("ElementKeyInt(%d): %w", i, err)
			}
			values[i] = val
		}
		return tftypes.NewValue(schema, values), nil
	}

	value, err := tftypes.ValueFromJSON(raw, schema) //nolint:staticcheck // terraform-plugin-go exposes no stable JSON decoder for tftypes.Value.
	if err != nil {
		return tftypes.Value{}, err
	}
	return value, nil
}

func isUnknownSentinel(raw []byte) bool {
	var sentinel struct {
		Unknown bool `json:"$terraformUnknown"`
	}
	if err := json.Unmarshal(raw, &sentinel); err != nil {
		return false
	}
	return sentinel.Unknown
}

func canonicalJSONFromValue(value tftypes.Value, schema tftypes.Type) (any, error) {
	if !value.IsKnown() {
		typeJSON, err := schema.MarshalJSON() //nolint:staticcheck // same JSON shape Terraform uses for dynamic type signatures.
		if err != nil {
			return nil, err
		}

		var typ any
		if err := json.Unmarshal(typeJSON, &typ); err != nil {
			return nil, err
		}

		return map[string]any{
			unknownSentinelKey: true,
			"type":             typ,
		}, nil
	}

	if value.IsNull() {
		return nil, nil
	}

	if schema.Is(tftypes.DynamicPseudoType) {
		valueJSON, err := canonicalJSONFromValue(value, value.Type())
		if err != nil {
			return nil, err
		}
		typeJSON, err := value.Type().MarshalJSON() //nolint:staticcheck
		if err != nil {
			return nil, err
		}
		var typ any
		if err := json.Unmarshal(typeJSON, &typ); err != nil {
			return nil, err
		}
		return map[string]any{"type": typ, "value": valueJSON}, nil
	}

	switch typ := value.Type().(type) {
	case tftypes.Object:
		var values map[string]tftypes.Value
		if err := value.As(&values); err != nil {
			return nil, err
		}

		out := make(map[string]any, len(values))
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next, err := canonicalJSONFromValue(values[key], typ.AttributeTypes[key])
			if err != nil {
				return nil, fmt.Errorf("attribute %q: %w", key, err)
			}
			out[key] = next
		}
		return out, nil

	case tftypes.Map:
		var values map[string]tftypes.Value
		if err := value.As(&values); err != nil {
			return nil, err
		}

		out := make(map[string]any, len(values))
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next, err := canonicalJSONFromValue(values[key], typ.ElementType)
			if err != nil {
				return nil, fmt.Errorf("map key %q: %w", key, err)
			}
			out[key] = next
		}
		return out, nil

	case tftypes.List:
		return canonicalJSONArrayFromValue(value, typ.ElementType)

	case tftypes.Set:
		return canonicalJSONArrayFromValue(value, typ.ElementType)

	case tftypes.Tuple:
		var values []tftypes.Value
		if err := value.As(&values); err != nil {
			return nil, err
		}
		if len(values) != len(typ.ElementTypes) {
			return nil, fmt.Errorf("tuple length %d does not match schema length %d", len(values), len(typ.ElementTypes))
		}

		out := make([]any, len(values))
		for i, element := range values {
			next, err := canonicalJSONFromValue(element, typ.ElementTypes[i])
			if err != nil {
				return nil, fmt.Errorf("tuple index %d: %w", i, err)
			}
			out[i] = next
		}
		return out, nil
	}

	switch {
	case value.Type().Is(tftypes.String):
		var out string
		if err := value.As(&out); err != nil {
			return nil, err
		}
		return out, nil
	case value.Type().Is(tftypes.Bool):
		var out bool
		if err := value.As(&out); err != nil {
			return nil, err
		}
		return out, nil
	case value.Type().Is(tftypes.Number):
		var out *big.Float
		if err := value.As(&out); err != nil {
			return nil, err
		}
		return json.Number(out.Text('f', -1)), nil
	default:
		return nil, fmt.Errorf("unsupported terraform type %s", value.Type())
	}
}

func canonicalJSONArrayFromValue(value tftypes.Value, elementType tftypes.Type) ([]any, error) {
	var values []tftypes.Value
	if err := value.As(&values); err != nil {
		return nil, err
	}

	out := make([]any, len(values))
	for i, element := range values {
		next, err := canonicalJSONFromValue(element, elementType)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out[i] = next
	}
	return out, nil
}
