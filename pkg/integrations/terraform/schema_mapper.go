package terraform

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func MapV5Schema(schema *tfprotov5.Schema) (SchemaRepresentation, error) {
	if schema == nil {
		return SchemaRepresentation{}, nil
	}
	block, err := mapV5Block(schema.Block)
	if err != nil {
		return SchemaRepresentation{}, err
	}
	return SchemaRepresentation{Version: schema.Version, Block: block}, nil
}

func MapV6Schema(schema *tfprotov6.Schema) (SchemaRepresentation, error) {
	if schema == nil {
		return SchemaRepresentation{}, nil
	}
	block, err := mapV6Block(schema.Block)
	if err != nil {
		return SchemaRepresentation{}, err
	}
	return SchemaRepresentation{Version: schema.Version, Block: block}, nil
}

func mapV5Block(block *tfprotov5.SchemaBlock) (Block, error) {
	if block == nil {
		return Block{}, nil
	}
	out := Block{Description: block.Description}
	if len(block.Attributes) > 0 {
		out.Attributes = make(map[string]Attribute, len(block.Attributes))
	}
	for _, attr := range block.Attributes {
		if attr == nil {
			continue
		}
		mapped, err := mapV5Attribute(attr)
		if err != nil {
			return Block{}, fmt.Errorf("attribute %q: %w", attr.Name, err)
		}
		out.Attributes[attr.Name] = mapped
	}
	if len(block.BlockTypes) > 0 {
		out.BlockTypes = make(map[string]NestedBlockType, len(block.BlockTypes))
	}
	for _, nested := range block.BlockTypes {
		if nested == nil {
			continue
		}
		mappedBlock, err := mapV5Block(nested.Block)
		if err != nil {
			return Block{}, fmt.Errorf("block %q: %w", nested.TypeName, err)
		}
		out.BlockTypes[nested.TypeName] = NestedBlockType{
			NestingMode: schemaNestedBlockMode(nested.Nesting.String()),
			Block:       mappedBlock,
			MinItems:    int(nested.MinItems),
			MaxItems:    int(nested.MaxItems),
		}
	}
	return out, nil
}

func mapV5Attribute(attr *tfprotov5.SchemaAttribute) (Attribute, error) {
	typeJSON, err := terraformTypeJSON(attr.Type)
	if err != nil {
		return Attribute{}, err
	}
	return Attribute{
		Type:        typeJSON,
		Description: attr.Description,
		Required:    attr.Required,
		Optional:    attr.Optional,
		Computed:    attr.Computed,
		Sensitive:   attr.Sensitive,
		WriteOnly:   attr.WriteOnly,
	}, nil
}

func mapV6Block(block *tfprotov6.SchemaBlock) (Block, error) {
	if block == nil {
		return Block{}, nil
	}
	out := Block{Description: block.Description}
	if len(block.Attributes) > 0 {
		out.Attributes = make(map[string]Attribute, len(block.Attributes))
	}
	for _, attr := range block.Attributes {
		if attr == nil {
			continue
		}
		mapped, err := mapV6Attribute(attr)
		if err != nil {
			return Block{}, fmt.Errorf("attribute %q: %w", attr.Name, err)
		}
		out.Attributes[attr.Name] = mapped
	}
	if len(block.BlockTypes) > 0 {
		out.BlockTypes = make(map[string]NestedBlockType, len(block.BlockTypes))
	}
	for _, nested := range block.BlockTypes {
		if nested == nil {
			continue
		}
		mappedBlock, err := mapV6Block(nested.Block)
		if err != nil {
			return Block{}, fmt.Errorf("block %q: %w", nested.TypeName, err)
		}
		out.BlockTypes[nested.TypeName] = NestedBlockType{
			NestingMode: schemaNestedBlockMode(nested.Nesting.String()),
			Block:       mappedBlock,
			MinItems:    int(nested.MinItems),
			MaxItems:    int(nested.MaxItems),
		}
	}
	return out, nil
}

func mapV6Attribute(attr *tfprotov6.SchemaAttribute) (Attribute, error) {
	out := Attribute{
		Description: attr.Description,
		Required:    attr.Required,
		Optional:    attr.Optional,
		Computed:    attr.Computed,
		Sensitive:   attr.Sensitive,
		WriteOnly:   attr.WriteOnly,
	}
	if attr.NestedType != nil {
		nested, err := mapV6NestedType(attr.NestedType)
		if err != nil {
			return Attribute{}, err
		}
		out.NestedType = nested
		return out, nil
	}
	typeJSON, err := terraformTypeJSON(attr.Type)
	if err != nil {
		return Attribute{}, err
	}
	out.Type = typeJSON
	return out, nil
}

func mapV6NestedType(object *tfprotov6.SchemaObject) (*NestedType, error) {
	nested := &NestedType{
		NestingMode: schemaObjectMode(object.Nesting.String()),
		Attributes:  make(map[string]Attribute, len(object.Attributes)),
	}
	for _, attr := range object.Attributes {
		if attr == nil {
			continue
		}
		mapped, err := mapV6Attribute(attr)
		if err != nil {
			return nil, fmt.Errorf("nested attribute %q: %w", attr.Name, err)
		}
		nested.Attributes[attr.Name] = mapped
	}
	return nested, nil
}

func terraformTypeJSON(typ tftypes.Type) (json.RawMessage, error) {
	if typ == nil {
		return nil, fmt.Errorf("terraform type is nil")
	}
	raw, err := typ.MarshalJSON() //nolint:staticcheck // Terraform CLI schema JSON uses the same type signature shape.
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func schemaNestedBlockMode(mode string) string {
	switch strings.ToLower(mode) {
	case "single", "group":
		return "single"
	case "list", "set", "map":
		return strings.ToLower(mode)
	default:
		return "invalid"
	}
}

func schemaObjectMode(mode string) string {
	switch strings.ToLower(mode) {
	case "single", "list", "set", "map":
		return strings.ToLower(mode)
	default:
		return "invalid"
	}
}
