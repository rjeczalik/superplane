package terraform

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestRuntimeSchemaLoaderLoad(t *testing.T) {
	factory := &fakeRuntimeFactory{runtime: &fakeProviderRuntime{schema: runtimeSchema(t)}}
	loader := NewRuntimeSchemaLoader(factory)

	schemas, err := loader.Load(context.Background(), talosConfig())
	require.NoError(t, err)
	assert.Equal(t, 1, factory.calls)

	provider := schemas.ProviderSchemas[talosConfig().Source]
	require.Contains(t, provider.ResourceSchemas, "talos_machine_secrets")
	translated, ok := TranslateBlock(provider.ResourceSchemas["talos_machine_secrets"].Block)
	require.True(t, ok)
	require.Len(t, translated.InputFields, 1)
	assert.True(t, factory.runtime.closed)
}

func TestRuntimeSchemaLoaderSchemaErrorClosesRuntime(t *testing.T) {
	rt := &fakeProviderRuntime{schemaErr: assert.AnError}
	_, err := NewRuntimeSchemaLoader(&fakeRuntimeFactory{runtime: rt}).Load(context.Background(), talosConfig())
	require.Error(t, err)
	assert.True(t, rt.closed)
}

func runtimeSchema(t *testing.T) *runtime.SchemaResponse {
	t.Helper()
	block := blockJSON(t, Block{Attributes: map[string]Attribute{
		"name": {Type: rawJSON(t, `"string"`), Required: true},
	}})
	return &runtime.SchemaResponse{
		Provider: runtime.ProviderSchemaBlock{Block: blockJSON(t, Block{})},
		Resources: map[string]runtime.SchemaBlock{
			"talos_machine_secrets": {Version: 1, Block: block},
		},
		DataSources: map[string]runtime.SchemaBlock{},
		Actions:     map[string]runtime.SchemaBlock{},
	}
}

type fakeRuntimeFactory struct {
	runtime *fakeProviderRuntime
	calls   int
	lastCfg config.TerraformProviderIntegration
}

func (f *fakeRuntimeFactory) RuntimeForProvider(ctx context.Context, cfg config.TerraformProviderIntegration) (runtime.ProviderRuntime, error) {
	f.calls++
	f.lastCfg = cfg
	return f.runtime, nil
}
