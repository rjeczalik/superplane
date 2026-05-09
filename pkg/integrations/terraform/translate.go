package terraform

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/superplanehq/superplane/pkg/configuration"
)

// TranslateResult holds the output of translating a Terraform Block into
// SuperPlane configuration fields.
type TranslateResult struct {
	InputFields    []configuration.Field
	OutputFields   []configuration.Field
	SensitiveAttrs map[string]struct{} // dotted paths
	SchemaHash     string              // SHA256 of canonical JSON of the source block
}

// TranslateBlock walks a Terraform Block and translates it into
// configuration.Fields. It returns ok=false if any unsupported type is
// encountered anywhere in the tree.
func TranslateBlock(block Block) (TranslateResult, bool) {
	canonical, err := canonicalJSON(block)
	if err != nil {
		return TranslateResult{}, false
	}
	schemaHash := fmt.Sprintf("%x", sha256.Sum256(canonical))

	inputFields := make([]configuration.Field, 0)
	outputFields := make([]configuration.Field, 0)
	sensitiveAttrs := make(map[string]struct{})

	ancestors := make(map[string]struct{})

	for name, attr := range block.Attributes {
		field, outField, ok := translateAttribute(name, attr, ancestors)
		if !ok {
			return TranslateResult{}, false
		}

		collectSensitiveAttributePaths(name, attr, sensitiveAttrs)

		// Computed-only attributes are excluded from input schema.
		if !attr.Computed || attr.Optional || attr.Required {
			inputFields = append(inputFields, field)
		}

		// Output schema includes all attributes, including computed-only.
		if outField != nil {
			outputFields = append(outputFields, *outField)
		}
	}

	for name, bt := range block.BlockTypes {
		field, outField, ok := translateNestedBlock(name, bt, ancestors)
		if !ok {
			return TranslateResult{}, false
		}
		collectSensitiveBlockPaths(name, bt.Block, sensitiveAttrs)

		inputFields = append(inputFields, field)
		if outField != nil {
			outputFields = append(outputFields, *outField)
		}
	}

	return TranslateResult{
		InputFields:    inputFields,
		OutputFields:   outputFields,
		SensitiveAttrs: sensitiveAttrs,
		SchemaHash:     schemaHash,
	}, true
}

func collectSensitiveAttributePaths(prefix string, attr Attribute, sensitive map[string]struct{}) {
	if attr.Sensitive || attr.WriteOnly {
		sensitive[prefix] = struct{}{}
	}
	if attr.NestedType == nil {
		return
	}
	for name, child := range attr.NestedType.Attributes {
		collectSensitiveAttributePaths(prefix+"."+name, child, sensitive)
	}
}

func collectSensitiveBlockPaths(prefix string, block Block, sensitive map[string]struct{}) {
	for name, attr := range block.Attributes {
		collectSensitiveAttributePaths(prefix+"."+name, attr, sensitive)
	}
	for name, child := range block.BlockTypes {
		collectSensitiveBlockPaths(prefix+"."+name, child.Block, sensitive)
	}
}

func translateAttribute(name string, attr Attribute, ancestors map[string]struct{}) (configuration.Field, *configuration.Field, bool) {
	field := configuration.Field{
		Name:        name,
		Label:       name,
		Description: attr.Description,
		Required:    attr.Required,
		Sensitive:   attr.Sensitive || attr.WriteOnly,
	}

	outField := field

	if attr.NestedType != nil {
		return translateNestedTypeAttribute(name, attr, ancestors)
	}

	tp, err := ParseType(attr.Type)
	if err != nil {
		return configuration.Field{}, nil, false
	}

	switch tp.Kind {
	case TypeKindPrimitive:
		switch tp.Primitive {
		case "string":
			field.Type = configuration.FieldTypeString
		case "bool":
			field.Type = configuration.FieldTypeBool
		case "number":
			field.Type = configuration.FieldTypeNumber
		default:
			return configuration.Field{}, nil, false
		}
	case TypeKindList:
		if tp.Element == nil {
			return configuration.Field{}, nil, false
		}
		if tp.Element.Kind == TypeKindPrimitive && tp.Element.Primitive == "bool" {
			return configuration.Field{}, nil, false
		}
		field.Type = configuration.FieldTypeList
		itemDef, ok := typeSpecToListItemDefinition(tp.Element)
		if !ok {
			return configuration.Field{}, nil, false
		}
		field.TypeOptions = &configuration.TypeOptions{
			List: &configuration.ListTypeOptions{
				ItemDefinition: itemDef,
			},
		}
	case TypeKindSet:
		if tp.Element == nil {
			return configuration.Field{}, nil, false
		}
		if tp.Element.Kind == TypeKindPrimitive && tp.Element.Primitive == "bool" {
			return configuration.Field{}, nil, false
		}
		field.Type = configuration.FieldTypeList
		itemDef, ok := typeSpecToListItemDefinition(tp.Element)
		if !ok {
			return configuration.Field{}, nil, false
		}
		field.TypeOptions = &configuration.TypeOptions{
			List: &configuration.ListTypeOptions{
				ItemDefinition: itemDef,
			},
		}
	case TypeKindMap:
		if tp.Element == nil {
			return configuration.Field{}, nil, false
		}
		if tp.Element.Kind == TypeKindObject {
			return configuration.Field{}, nil, false
		}
		field.Type = configuration.FieldTypeObject
		if tp.Element.Kind == TypeKindPrimitive && tp.Element.Primitive == "string" {
			// map(string) -> object with no schema (free-form JSON object editor)
			field.TypeOptions = &configuration.TypeOptions{
				Object: &configuration.ObjectTypeOptions{},
			}
		} else {
			// map of other primitives: still object with no schema
			field.TypeOptions = &configuration.TypeOptions{
				Object: &configuration.ObjectTypeOptions{},
			}
		}
	case TypeKindObject:
		field.Type = configuration.FieldTypeObject
		schema, ok := typeSpecToObjectSchema(&tp, ancestors)
		if !ok {
			return configuration.Field{}, nil, false
		}
		field.TypeOptions = &configuration.TypeOptions{
			Object: &configuration.ObjectTypeOptions{
				Schema: schema,
			},
		}
	case TypeKindTuple, TypeKindDynamic:
		return configuration.Field{}, nil, false
	default:
		return configuration.Field{}, nil, false
	}

	return field, &outField, true
}

func translateNestedTypeAttribute(name string, attr Attribute, ancestors map[string]struct{}) (configuration.Field, *configuration.Field, bool) {
	field := configuration.Field{
		Name:        name,
		Label:       name,
		Description: attr.Description,
		Required:    attr.Required,
		Sensitive:   attr.Sensitive || attr.WriteOnly,
	}

	outField := field

	childAncestors := make(map[string]struct{}, len(ancestors)+1)
	for k := range ancestors {
		childAncestors[k] = struct{}{}
	}
	childAncestors[name] = struct{}{}

	switch attr.NestedType.NestingMode {
	case "single":
		field.Type = configuration.FieldTypeObject
		schema, ok := blockAttributesToObjectSchema(attr.NestedType.Attributes, childAncestors)
		if !ok {
			return configuration.Field{}, nil, false
		}
		field.TypeOptions = &configuration.TypeOptions{
			Object: &configuration.ObjectTypeOptions{
				Schema: schema,
			},
		}
	case "list":
		field.Type = configuration.FieldTypeList
		schema, ok := blockAttributesToObjectSchema(attr.NestedType.Attributes, childAncestors)
		if !ok {
			return configuration.Field{}, nil, false
		}
		field.TypeOptions = &configuration.TypeOptions{
			List: &configuration.ListTypeOptions{
				ItemDefinition: &configuration.ListItemDefinition{
					Type:   configuration.FieldTypeObject,
					Schema: schema,
				},
			},
		}
	case "set":
		field.Type = configuration.FieldTypeList
		schema, ok := blockAttributesToObjectSchema(attr.NestedType.Attributes, childAncestors)
		if !ok {
			return configuration.Field{}, nil, false
		}
		field.TypeOptions = &configuration.TypeOptions{
			List: &configuration.ListTypeOptions{
				ItemDefinition: &configuration.ListItemDefinition{
					Type:   configuration.FieldTypeObject,
					Schema: schema,
				},
			},
		}
	case "map":
		return configuration.Field{}, nil, false
	default:
		return configuration.Field{}, nil, false
	}

	return field, &outField, true
}

func translateNestedBlock(name string, bt NestedBlockType, ancestors map[string]struct{}) (configuration.Field, *configuration.Field, bool) {
	field := configuration.Field{
		Name:        name,
		Label:       name,
		Description: bt.Block.Description,
		Required:    bt.MinItems > 0,
	}

	outField := field

	childAncestors := make(map[string]struct{}, len(ancestors)+1)
	for k := range ancestors {
		childAncestors[k] = struct{}{}
	}
	childAncestors[name] = struct{}{}

	switch bt.NestingMode {
	case "single":
		field.Type = configuration.FieldTypeObject
		schema, ok := blockAttributesToObjectSchema(bt.Block.Attributes, childAncestors)
		if !ok {
			return configuration.Field{}, nil, false
		}
		// Include nested blocks inside the nested block as well.
		for childName, childBt := range bt.Block.BlockTypes {
			childField, _, ok := translateNestedBlock(childName, childBt, childAncestors)
			if !ok {
				return configuration.Field{}, nil, false
			}
			schema = append(schema, childField)
		}
		field.TypeOptions = &configuration.TypeOptions{
			Object: &configuration.ObjectTypeOptions{
				Schema: schema,
			},
		}
	case "list":
		field.Type = configuration.FieldTypeList
		schema, ok := blockAttributesToObjectSchema(bt.Block.Attributes, childAncestors)
		if !ok {
			return configuration.Field{}, nil, false
		}
		for childName, childBt := range bt.Block.BlockTypes {
			childField, _, ok := translateNestedBlock(childName, childBt, childAncestors)
			if !ok {
				return configuration.Field{}, nil, false
			}
			schema = append(schema, childField)
		}
		field.TypeOptions = &configuration.TypeOptions{
			List: &configuration.ListTypeOptions{
				ItemDefinition: &configuration.ListItemDefinition{
					Type:   configuration.FieldTypeObject,
					Schema: schema,
				},
				MaxItems: maxItemsPtr(bt.MaxItems),
			},
		}
	case "set":
		field.Type = configuration.FieldTypeList
		schema, ok := blockAttributesToObjectSchema(bt.Block.Attributes, childAncestors)
		if !ok {
			return configuration.Field{}, nil, false
		}
		for childName, childBt := range bt.Block.BlockTypes {
			childField, _, ok := translateNestedBlock(childName, childBt, childAncestors)
			if !ok {
				return configuration.Field{}, nil, false
			}
			schema = append(schema, childField)
		}
		field.TypeOptions = &configuration.TypeOptions{
			List: &configuration.ListTypeOptions{
				ItemDefinition: &configuration.ListItemDefinition{
					Type:   configuration.FieldTypeObject,
					Schema: schema,
				},
				MaxItems: maxItemsPtr(bt.MaxItems),
			},
		}
	case "map":
		return configuration.Field{}, nil, false
	default:
		return configuration.Field{}, nil, false
	}

	return field, &outField, true
}

func maxItemsPtr(maxItems int) *int {
	if maxItems <= 0 {
		return nil
	}
	return &maxItems
}

func blockAttributesToObjectSchema(attrs map[string]Attribute, ancestors map[string]struct{}) ([]configuration.Field, bool) {
	schema := make([]configuration.Field, 0, len(attrs))
	for name, attr := range attrs {
		if _, ok := ancestors[name]; ok {
			return nil, false
		}

		field, _, ok := translateAttribute(name, attr, ancestors)
		if !ok {
			return nil, false
		}
		schema = append(schema, field)
	}
	return schema, true
}

func typeSpecToListItemDefinition(tp *TypeSpec) (*configuration.ListItemDefinition, bool) {
	if tp == nil {
		return nil, false
	}
	switch tp.Kind {
	case TypeKindPrimitive:
		switch tp.Primitive {
		case "string":
			return &configuration.ListItemDefinition{Type: configuration.FieldTypeString}, true
		case "number":
			return &configuration.ListItemDefinition{Type: configuration.FieldTypeNumber}, true
		case "bool":
			return &configuration.ListItemDefinition{Type: configuration.FieldTypeBool}, true
		default:
			return nil, false
		}
	case TypeKindObject:
		schema, ok := typeSpecToObjectSchema(tp, make(map[string]struct{}))
		if !ok {
			return nil, false
		}
		return &configuration.ListItemDefinition{
			Type:   configuration.FieldTypeObject,
			Schema: schema,
		}, true
	case TypeKindList:
		// list(list(...)) is not supported in MVP
		return nil, false
	case TypeKindSet:
		return nil, false
	case TypeKindMap:
		return nil, false
	default:
		return nil, false
	}
}

func typeSpecToObjectSchema(tp *TypeSpec, ancestors map[string]struct{}) ([]configuration.Field, bool) {
	if tp.Kind != TypeKindObject {
		return nil, false
	}
	schema := make([]configuration.Field, 0, len(tp.Attributes))
	for name, attrSpec := range tp.Attributes {
		if _, ok := ancestors[name]; ok {
			return nil, false
		}

		field := configuration.Field{
			Name:  name,
			Label: name,
		}
		switch attrSpec.Kind {
		case TypeKindPrimitive:
			switch attrSpec.Primitive {
			case "string":
				field.Type = configuration.FieldTypeString
			case "bool":
				field.Type = configuration.FieldTypeBool
			case "number":
				field.Type = configuration.FieldTypeNumber
			default:
				return nil, false
			}
		case TypeKindObject:
			field.Type = configuration.FieldTypeObject
			childAncestors := make(map[string]struct{}, len(ancestors)+1)
			for k := range ancestors {
				childAncestors[k] = struct{}{}
			}
			childAncestors[name] = struct{}{}
			nestedSchema, ok := typeSpecToObjectSchema(&attrSpec, childAncestors)
			if !ok {
				return nil, false
			}
			field.TypeOptions = &configuration.TypeOptions{
				Object: &configuration.ObjectTypeOptions{
					Schema: nestedSchema,
				},
			}
		case TypeKindList:
			field.Type = configuration.FieldTypeList
			itemDef, ok := typeSpecToListItemDefinition(attrSpec.Element)
			if !ok {
				return nil, false
			}
			field.TypeOptions = &configuration.TypeOptions{
				List: &configuration.ListTypeOptions{
					ItemDefinition: itemDef,
				},
			}
		case TypeKindSet:
			field.Type = configuration.FieldTypeList
			itemDef, ok := typeSpecToListItemDefinition(attrSpec.Element)
			if !ok {
				return nil, false
			}
			field.TypeOptions = &configuration.TypeOptions{
				List: &configuration.ListTypeOptions{
					ItemDefinition: itemDef,
				},
			}
		case TypeKindMap:
			if attrSpec.Element != nil && attrSpec.Element.Kind == TypeKindObject {
				return nil, false
			}
			field.Type = configuration.FieldTypeObject
			field.TypeOptions = &configuration.TypeOptions{
				Object: &configuration.ObjectTypeOptions{},
			}
		case TypeKindTuple, TypeKindDynamic:
			return nil, false
		default:
			return nil, false
		}
		schema = append(schema, field)
	}
	return schema, true
}

func canonicalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
