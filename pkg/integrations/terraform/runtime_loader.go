package terraform

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

type ConfiguredRuntimeFactory interface {
	RuntimeForProvider(ctx context.Context, cfg config.TerraformProviderIntegration) (runtime.ProviderRuntime, error)
}

type RuntimeSchemaLoader struct {
	runtimeFactory ConfiguredRuntimeFactory
}

func NewRuntimeSchemaLoader(factory ConfiguredRuntimeFactory) *RuntimeSchemaLoader {
	return &RuntimeSchemaLoader{runtimeFactory: factory}
}

func (l *RuntimeSchemaLoader) Load(ctx context.Context, cfg config.TerraformProviderIntegration) (*ProviderSchemasFile, error) {
	rt, err := l.runtimeFactory.RuntimeForProvider(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer rt.Close()

	schema, err := rt.Schema(ctx)
	if err != nil {
		return nil, err
	}

	provider, err := providerSchemaFromRuntime(schema)
	if err != nil {
		return nil, err
	}
	return &ProviderSchemasFile{
		FormatVersion: "1.0",
		ProviderSchemas: map[string]ProviderSchema{
			cfg.Source: provider,
		},
	}, nil
}

func providerSchemaFromRuntime(schema *runtime.SchemaResponse) (ProviderSchema, error) {
	if schema == nil {
		return ProviderSchema{}, fmt.Errorf("runtime schema response is nil")
	}
	providerBlock, err := blockFromJSON(schema.Provider.Block)
	if err != nil {
		return ProviderSchema{}, fmt.Errorf("provider schema: %w", err)
	}
	out := ProviderSchema{
		Provider:          SchemaRepresentation{Block: providerBlock},
		ResourceSchemas:   make(map[string]SchemaRepresentation, len(schema.Resources)),
		DataSourceSchemas: make(map[string]SchemaRepresentation, len(schema.DataSources)),
	}
	for name, block := range schema.Resources {
		mapped, err := blockFromJSON(block.Block)
		if err != nil {
			return ProviderSchema{}, fmt.Errorf("resource %q schema: %w", name, err)
		}
		out.ResourceSchemas[name] = SchemaRepresentation{Version: block.Version, Block: mapped}
	}
	for name, block := range schema.DataSources {
		mapped, err := blockFromJSON(block.Block)
		if err != nil {
			return ProviderSchema{}, fmt.Errorf("data source %q schema: %w", name, err)
		}
		out.DataSourceSchemas[name] = SchemaRepresentation{Version: block.Version, Block: mapped}
	}
	return out, nil
}

func blockFromJSON(raw []byte) (Block, error) {
	if len(raw) == 0 {
		return Block{}, nil
	}
	var block Block
	if err := json.Unmarshal(raw, &block); err != nil {
		return Block{}, err
	}
	return block, nil
}
