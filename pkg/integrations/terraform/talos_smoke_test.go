package terraform

import (
	"context"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/crypto"
	"github.com/superplanehq/superplane/pkg/registry"
)

func TestTalosProviderStartupSmoke(t *testing.T) {
	if os.Getenv("SUPERPLANE_TERRAFORM_SMOKE") != "1" {
		t.Skip("set SUPERPLANE_TERRAFORM_SMOKE=1 to run")
	}

	cacheDir := t.TempDir()
	err := RegisterConfiguredProviders(context.Background(), RegisterOptions{
		Providers: []config.TerraformProviderIntegration{{
			Name:    "talos",
			Label:   "Talos",
			Source:  "registry.terraform.io/siderolabs/talos",
			Version: "0.11.0",
			Expose:  config.TerraformProviderExpose{Resources: "*", DataSources: "*"},
		}},
		CacheDir: cacheDir,
		Logger:   log.NewEntry(log.New()),
	})
	require.NoError(t, err)

	r, err := registry.NewRegistryWithOptions(registry.RegistryOptions{Encryptor: crypto.NewNoOpEncryptor()})
	require.NoError(t, err)

	for _, name := range []string{
		"talos.machineSecrets.create",
		"talos.machineSecrets.read",
		"talos.machineSecrets.update",
		"talos.machineSecrets.delete",
		"talos.machineConfiguration.data",
		"talos.machineConfigurationApply.create",
		"talos.machineBootstrap.create",
		"talos.clusterHealth.data",
		"talos.clusterKubeconfig.create",
		"talos.clusterKubeconfig.data",
	} {
		action, err := r.GetIntegrationAction("talos", name)
		require.NoError(t, err, "action %s not found", name)
		require.NotNil(t, action)
	}
}
