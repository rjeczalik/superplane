package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerraformProviderIntegrations(t *testing.T) {
	t.Run("empty env returns nil", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", "")
		providers, err := TerraformProviderIntegrations()
		require.NoError(t, err)
		assert.Nil(t, providers)
	})

	t.Run("valid JSON parses", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{
			"name":"talos",
			"label":"Talos",
			"source":"registry.terraform.io/siderolabs/talos",
			"version":"0.11.0"
		}]`)
		providers, err := TerraformProviderIntegrations()
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "talos", providers[0].Name)
		assert.Equal(t, "Talos", providers[0].Label)
		assert.Equal(t, "registry.terraform.io/siderolabs/talos", providers[0].Source)
		assert.Equal(t, "0.11.0", providers[0].Version)
		assert.Equal(t, []string{}, providers[0].Expose.Resources)
		assert.Equal(t, "*", providers[0].Expose.DataSources)
	})

	t.Run("missing name returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"source":"registry.terraform.io/siderolabs/talos","version":"0.11.0"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("missing source returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","version":"0.11.0"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source")
	})

	t.Run("missing version returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"registry.terraform.io/siderolabs/talos"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version")
	})

	t.Run("name with dot returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"tal.os","source":"registry.terraform.io/siderolabs/talos","version":"0.11.0"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("name not matching regex returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"Talos","source":"registry.terraform.io/siderolabs/talos","version":"0.11.0"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("version latest returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"registry.terraform.io/siderolabs/talos","version":"latest"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version")
	})

	t.Run("version with constraint operators returns error", func(t *testing.T) {
		for _, v := range []string{">=0.11.0", "~>0.11.0", ">0.11.0", "<0.11.0", ">= 0.11.0", "0.11.0, 0.12.0"} {
			t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"registry.terraform.io/siderolabs/talos","version":"`+v+`"}]`)
			_, err := TerraformProviderIntegrations()
			require.Error(t, err, "version %q should be rejected", v)
			assert.Contains(t, err.Error(), "version")
		}
	})

	t.Run("version not parseable as exact SemVer returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"registry.terraform.io/siderolabs/talos","version":"0.11"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version")
	})

	t.Run("source not matching format returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"invalid","version":"0.11.0"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source")
	})

	t.Run("source host not in allowlist returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"example.com/siderolabs/talos","version":"0.11.0"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source")
	})

	t.Run("duplicate names returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"registry.terraform.io/siderolabs/talos","version":"0.11.0"},{"name":"talos","source":"registry.terraform.io/siderolabs/talos","version":"0.11.1"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate")
	})

	t.Run("name must match provider source type", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos_prod","source":"registry.terraform.io/siderolabs/talos","version":"0.11.0"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider type")
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `not json`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
	})

	t.Run("defaults for label, description, icon, expose", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{
			"name":"talos",
			"source":"registry.terraform.io/siderolabs/talos",
			"version":"0.11.0"
		}]`)
		providers, err := TerraformProviderIntegrations()
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "talos", providers[0].Label)
		assert.Equal(t, "Terraform provider registry.terraform.io/siderolabs/talos", providers[0].Description)
		assert.Equal(t, "", providers[0].Icon)
		assert.Equal(t, []string{}, providers[0].Expose.Resources)
		assert.Equal(t, "*", providers[0].Expose.DataSources)
	})

	t.Run("explicit wildcard resources remains explicit", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{
			"name":"talos",
			"source":"registry.terraform.io/siderolabs/talos",
			"version":"0.11.0",
			"expose": {"resources": "*"}
		}]`)
		providers, err := TerraformProviderIntegrations()
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "*", providers[0].Expose.Resources)
		assert.Equal(t, "*", providers[0].Expose.DataSources)
	})

	t.Run("expose ephemeralResources parsed but ignored", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{
			"name":"talos",
			"source":"registry.terraform.io/siderolabs/talos",
			"version":"0.11.0",
			"expose": {
				"resources": "*",
				"dataSources": ["talos_client_configuration"],
				"ephemeralResources": ["ignored"]
			}
		}]`)
		providers, err := TerraformProviderIntegrations()
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "*", providers[0].Expose.Resources)
		assert.Equal(t, []any{"talos_client_configuration"}, providers[0].Expose.DataSources)
	})

	t.Run("valid SemVer with prerelease", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"registry.terraform.io/siderolabs/talos","version":"0.11.0-beta.1"}]`)
		providers, err := TerraformProviderIntegrations()
		require.NoError(t, err)
		assert.Equal(t, "0.11.0-beta.1", providers[0].Version)
	})

	t.Run("valid SemVer with build metadata", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"registry.terraform.io/siderolabs/talos","version":"0.11.0+build.123"}]`)
		providers, err := TerraformProviderIntegrations()
		require.NoError(t, err)
		assert.Equal(t, "0.11.0+build.123", providers[0].Version)
	})

	t.Run("source with two parts defaults host", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"siderolabs/talos","version":"0.11.0"}]`)
		providers, err := TerraformProviderIntegrations()
		require.NoError(t, err)
		assert.Equal(t, "registry.terraform.io/siderolabs/talos", providers[0].Source)
	})

	t.Run("empty source namespace returns error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{"name":"talos","source":"registry.terraform.io//talos","version":"0.11.0"}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source")
	})

	t.Run("invalid expose values return error", func(t *testing.T) {
		t.Setenv("TERRAFORM_PROVIDER_INTEGRATIONS", `[{
			"name":"talos",
			"source":"registry.terraform.io/siderolabs/talos",
			"version":"0.11.0",
			"expose":{"resources":123,"dataSources":{"bad":true}}
		}]`)
		_, err := TerraformProviderIntegrations()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expose")
	})
}
