package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerraformProviderCacheDir(t *testing.T) {
	t.Setenv("TERRAFORM_PROVIDER_CACHE_DIR", "/tmp/tfp")
	assert.Equal(t, "/tmp/tfp", TerraformProviderCacheDir())

	t.Setenv("TERRAFORM_PROVIDER_CACHE_DIR", "")
	assert.Equal(t, "/var/lib/superplane/tfproviders", TerraformProviderCacheDir())
}

func TestTerraformExecutionTimeoutDefault(t *testing.T) {
	t.Setenv("TERRAFORM_EXECUTION_TIMEOUT_DEFAULT", "10m")
	assert.Equal(t, 10*time.Minute, TerraformExecutionTimeoutDefault())

	t.Setenv("TERRAFORM_EXECUTION_TIMEOUT_DEFAULT", "")
	assert.Equal(t, 30*time.Minute, TerraformExecutionTimeoutDefault())

	t.Setenv("TERRAFORM_EXECUTION_TIMEOUT_DEFAULT", "garbage")
	_, err := TerraformExecutionTimeoutDefaultE()
	require.Error(t, err)
}
