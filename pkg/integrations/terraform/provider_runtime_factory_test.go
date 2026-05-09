package terraform

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/config"
	tfplugin "github.com/superplanehq/superplane/pkg/integrations/terraform/plugin"
	tfregistry "github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestProviderRuntimeFactoryResolvesExtractsAndCreatesRuntime(t *testing.T) {
	archive := runtimeFactoryProviderZip(t, runtimeFactoryZipEntry{
		name: "terraform-provider-talos_0.11.0",
		mode: 0o755,
		body: "provider",
	})
	resolver := &recordingProviderResolver{
		pkg: &tfregistry.ProviderPackage{
			Protocols: []string{"6.0"},
			SHA256:    tfplugin.BytesSHA256(archive),
			Archive:   archive,
		},
	}
	rt := &fakeProviderRuntime{schema: runtimeSchema(t)}
	var gotReq RuntimeLaunchRequest
	factory, err := NewProviderRuntimeFactory(ProviderRuntimeFactoryOptions{
		Resolver:            resolver,
		CacheDir:            t.TempDir(),
		PluginIsolationMode: "disabled",
		RuntimeCreator: func(ctx context.Context, req RuntimeLaunchRequest) (runtime.ProviderRuntime, error) {
			gotReq = req
			return rt, nil
		},
	})
	require.NoError(t, err)

	got, err := factory.RuntimeForProvider(context.Background(), config.TerraformProviderIntegration{
		Name:    "talos",
		Source:  "registry.terraform.io/siderolabs/talos",
		Version: "0.11.0",
	})
	require.NoError(t, err)
	assert.Same(t, rt, got)
	assert.Equal(t, "registry.terraform.io/siderolabs/talos", resolver.source.String())
	assert.Equal(t, "0.11.0", resolver.version.String())
	assert.Equal(t, []string{"6.0"}, gotReq.Protocols)
	assert.Equal(t, "talos", gotReq.ProviderName)
	assert.Equal(t, "registry.terraform.io/siderolabs/talos", gotReq.ProviderSource)
	assert.Equal(t, "0.11.0", gotReq.ProviderVersion)
	assert.Equal(t, tfplugin.BytesSHA256([]byte("provider")), gotReq.ExecutableSHA256)
	assert.Equal(t, tfplugin.EgressUnisolated, gotReq.Isolation.Egress)
	require.NotEmpty(t, gotReq.BinaryPath)

	info, err := os.Stat(gotReq.BinaryPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o500), info.Mode().Perm())
}

func TestProviderRuntimeFactoryRejectsGPGSkipInProduction(t *testing.T) {
	_, err := NewProviderRuntimeFactory(ProviderRuntimeFactoryOptions{
		CacheDir:      t.TempDir(),
		AppEnv:        "production",
		GPGVerifyMode: "skip",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot skip Terraform provider GPG verification in production")
}

func TestProviderRuntimeFactoryRejectsUnisolatedPluginsInProduction(t *testing.T) {
	_, err := NewProviderRuntimeFactory(ProviderRuntimeFactoryOptions{
		CacheDir:            t.TempDir(),
		AppEnv:              "production",
		PluginIsolationMode: "disabled",
		RuntimeCreator:      func(context.Context, RuntimeLaunchRequest) (runtime.ProviderRuntime, error) { return nil, nil },
		Resolver:            &recordingProviderResolver{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot disable Terraform provider process isolation in production")
}

type recordingProviderResolver struct {
	source  tfregistry.ProviderSource
	version tfregistry.ProviderVersion
	pkg     *tfregistry.ProviderPackage
	err     error
}

func (r *recordingProviderResolver) ResolveProvider(ctx context.Context, source tfregistry.ProviderSource, version tfregistry.ProviderVersion) (*tfregistry.ProviderPackage, error) {
	r.source = source
	r.version = version
	return r.pkg, r.err
}

type runtimeFactoryZipEntry struct {
	name string
	mode os.FileMode
	body string
}

func runtimeFactoryProviderZip(t *testing.T, entries ...runtimeFactoryZipEntry) []byte {
	t.Helper()

	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		header.SetMode(entry.mode)
		file, err := writer.CreateHeader(header)
		require.NoError(t, err)
		_, err = file.Write([]byte(entry.body))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return out.Bytes()
}
