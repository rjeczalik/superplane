package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeOutputsHash(t *testing.T) {
	left := map[string]any{"name": "server", "size": float64(2), "tags": []any{"blue", "prod"}}
	right := map[string]any{"tags": []any{"blue", "prod"}, "size": float64(2), "name": "server"}

	leftHash, err := ComputeOutputsHash(left, nil)
	require.NoError(t, err)
	rightHash, err := ComputeOutputsHash(right, nil)
	require.NoError(t, err)
	assert.Equal(t, leftHash, rightHash)

	changedHash, err := ComputeOutputsHash(map[string]any{"name": "server", "size": float64(3)}, nil)
	require.NoError(t, err)
	assert.NotEqual(t, leftHash, changedHash)
}

func TestComputeOutputsHashChangedFields(t *testing.T) {
	base := map[string]any{"name": "server", "size": float64(2), "region": "iad"}
	changedIgnored := map[string]any{"name": "server", "size": float64(3), "region": "iad"}

	baseHash, err := ComputeOutputsHash(base, []string{"name", "region"})
	require.NoError(t, err)
	ignoredHash, err := ComputeOutputsHash(changedIgnored, []string{"region", "name"})
	require.NoError(t, err)
	assert.Equal(t, baseHash, ignoredHash)

	changedHash, err := ComputeOutputsHash(map[string]any{"name": "server-2", "size": float64(3), "region": "iad"}, []string{"name", "region"})
	require.NoError(t, err)
	assert.NotEqual(t, baseHash, changedHash)
}
