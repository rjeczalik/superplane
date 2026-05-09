package terraform

import (
	"encoding/json"
	"fmt"
)

// ProviderSchemasFile is the top-level structure returned by
// `terraform providers schema -json`.
type ProviderSchemasFile struct {
	FormatVersion   string                    `json:"format_version"`
	ProviderSchemas map[string]ProviderSchema `json:"provider_schemas"`
}

// ProviderSchema holds the schemas for a single provider.
type ProviderSchema struct {
	Provider                 SchemaRepresentation            `json:"provider"`
	ResourceSchemas          map[string]SchemaRepresentation `json:"resource_schemas"`
	DataSourceSchemas        map[string]SchemaRepresentation `json:"data_source_schemas"`
	EphemeralResourceSchemas map[string]SchemaRepresentation `json:"ephemeral_resource_schemas"`
}

// SchemaRepresentation is the schema for a resource, data source, or provider.
type SchemaRepresentation struct {
	Version int64 `json:"version"`
	Block   Block `json:"block"`
}

// Block describes the attributes and nested blocks of a schema.
type Block struct {
	Description string                     `json:"description,omitempty"`
	Attributes  map[string]Attribute       `json:"attributes,omitempty"`
	BlockTypes  map[string]NestedBlockType `json:"block_types,omitempty"`
}

// Attribute describes a single attribute within a Block.
type Attribute struct {
	Type        json.RawMessage `json:"type"`
	NestedType  *NestedType     `json:"nested_type,omitempty"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Optional    bool            `json:"optional,omitempty"`
	Computed    bool            `json:"computed,omitempty"`
	Sensitive   bool            `json:"sensitive,omitempty"`
	WriteOnly   bool            `json:"write_only,omitempty"`
}

// NestedType describes a nested attribute type (e.g. object with nested attrs).
type NestedType struct {
	NestingMode string               `json:"nesting_mode"` // single, list, set, map
	Attributes  map[string]Attribute `json:"attributes"`
}

// NestedBlockType describes a nested block within a Block.
type NestedBlockType struct {
	NestingMode string `json:"nesting_mode"`
	Block       Block  `json:"block"`
	MinItems    int    `json:"min_items,omitempty"`
	MaxItems    int    `json:"max_items,omitempty"`
}

// TypeSpec is a parsed representation of a Terraform type.
type TypeSpec struct {
	Kind       TypeKind
	Primitive  string              // for primitive kinds: "string", "bool", "number"
	Element    *TypeSpec           // for list, set, map
	Attributes map[string]TypeSpec // for object
	TupleElems []TypeSpec          // for tuple
}

// TypeKind enumerates the kinds of Terraform types we recognise.
type TypeKind int

const (
	TypeKindUnknown TypeKind = iota
	TypeKindPrimitive
	TypeKindList
	TypeKindSet
	TypeKindMap
	TypeKindObject
	TypeKindTuple
	TypeKindDynamic
)

// ParseProviderSchemas parses the raw JSON output of
// `terraform providers schema -json`.
func ParseProviderSchemas(raw []byte) (*ProviderSchemasFile, error) {
	var file ProviderSchemasFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("unmarshal provider schemas: %w", err)
	}
	return &file, nil
}

// ParseType parses a Terraform type encoded as JSON.
// Terraform types are either a string (e.g. "string") or an array
// (e.g. ["list","string"], ["object",{"a":"string"}]).
func ParseType(raw json.RawMessage) (TypeSpec, error) {
	if len(raw) == 0 {
		return TypeSpec{}, fmt.Errorf("empty type")
	}

	// String primitive: "string", "bool", "number", "dynamic"
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		switch str {
		case "string", "bool", "number":
			return TypeSpec{Kind: TypeKindPrimitive, Primitive: str}, nil
		case "dynamic":
			return TypeSpec{Kind: TypeKindDynamic}, nil
		default:
			return TypeSpec{}, fmt.Errorf("unknown primitive type %q", str)
		}
	}

	// Array form: [kind, ...]
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return TypeSpec{}, fmt.Errorf("type is neither string nor array: %w", err)
	}
	if len(arr) == 0 {
		return TypeSpec{}, fmt.Errorf("empty type array")
	}

	var kindStr string
	if err := json.Unmarshal(arr[0], &kindStr); err != nil {
		return TypeSpec{}, fmt.Errorf("type array first element is not a string: %w", err)
	}

	switch kindStr {
	case "list":
		if len(arr) != 2 {
			return TypeSpec{}, fmt.Errorf("list type expects 2 elements, got %d", len(arr))
		}
		elem, err := ParseType(arr[1])
		if err != nil {
			return TypeSpec{}, fmt.Errorf("list element: %w", err)
		}
		return TypeSpec{Kind: TypeKindList, Element: &elem}, nil
	case "set":
		if len(arr) != 2 {
			return TypeSpec{}, fmt.Errorf("set type expects 2 elements, got %d", len(arr))
		}
		elem, err := ParseType(arr[1])
		if err != nil {
			return TypeSpec{}, fmt.Errorf("set element: %w", err)
		}
		return TypeSpec{Kind: TypeKindSet, Element: &elem}, nil
	case "map":
		if len(arr) != 2 {
			return TypeSpec{}, fmt.Errorf("map type expects 2 elements, got %d", len(arr))
		}
		elem, err := ParseType(arr[1])
		if err != nil {
			return TypeSpec{}, fmt.Errorf("map element: %w", err)
		}
		return TypeSpec{Kind: TypeKindMap, Element: &elem}, nil
	case "object":
		if len(arr) != 2 {
			return TypeSpec{}, fmt.Errorf("object type expects 2 elements, got %d", len(arr))
		}
		var attrs map[string]json.RawMessage
		if err := json.Unmarshal(arr[1], &attrs); err != nil {
			return TypeSpec{}, fmt.Errorf("object attributes: %w", err)
		}
		result := TypeSpec{
			Kind:       TypeKindObject,
			Attributes: make(map[string]TypeSpec, len(attrs)),
		}
		for name, attrRaw := range attrs {
			attrSpec, err := ParseType(attrRaw)
			if err != nil {
				return TypeSpec{}, fmt.Errorf("object attribute %q: %w", name, err)
			}
			result.Attributes[name] = attrSpec
		}
		return result, nil
	case "tuple":
		if len(arr) != 2 {
			return TypeSpec{}, fmt.Errorf("tuple type expects 2 elements, got %d", len(arr))
		}
		var elems []json.RawMessage
		if err := json.Unmarshal(arr[1], &elems); err != nil {
			return TypeSpec{}, fmt.Errorf("tuple elements: %w", err)
		}
		result := TypeSpec{
			Kind:       TypeKindTuple,
			TupleElems: make([]TypeSpec, 0, len(elems)),
		}
		for i, elemRaw := range elems {
			elemSpec, err := ParseType(elemRaw)
			if err != nil {
				return TypeSpec{}, fmt.Errorf("tuple element %d: %w", i, err)
			}
			result.TupleElems = append(result.TupleElems, elemSpec)
		}
		return result, nil
	default:
		return TypeSpec{}, fmt.Errorf("unknown type kind %q", kindStr)
	}
}
