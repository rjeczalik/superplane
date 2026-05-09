package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testTerraformSecretWriter struct {
	values map[string][]byte
}

func (w *testTerraformSecretWriter) SetSecret(name string, value []byte) error {
	if w.values == nil {
		w.values = map[string][]byte{}
	}
	w.values[name] = value
	return nil
}

func TestSanitizeTerraformOutputsSeparatesSanitizedPayloadFromStableHashInput(t *testing.T) {
	sensitive := map[string]struct{}{"credentials.password": {}}
	payload := map[string]any{
		"id": "resource-1",
		"credentials": map[string]any{
			"username": "admin",
			"password": "secret-value",
		},
	}

	writer := &testTerraformSecretWriter{}
	first, firstHashInput, err := SanitizeTerraformOutputs(
		"canvas-1",
		"node-1",
		"resource-1",
		"operation-1",
		writer,
		payload,
		sensitive,
	)
	require.NoError(t, err)

	second, secondHashInput, err := SanitizeTerraformOutputs(
		"canvas-1",
		"node-1",
		"resource-1",
		"operation-2",
		writer,
		payload,
		sensitive,
	)
	require.NoError(t, err)

	require.NotContains(t, first, "secret-value")
	require.NotContains(t, firstHashInput, "secret-value")
	require.Equal(t, firstHashInput, secondHashInput)
	require.NotEqual(t,
		first["credentials"].(map[string]any)["password"],
		second["credentials"].(map[string]any)["password"],
	)
	require.Len(t, writer.values, 2)
}
