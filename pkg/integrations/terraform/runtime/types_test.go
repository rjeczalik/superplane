package runtime_test

import (
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestDiagnosticSeverity(t *testing.T) {
	if runtime.DiagError != 0 {
		t.Error("DiagError should be 0")
	}
	if runtime.DiagWarning != 1 {
		t.Error("DiagWarning should be 1")
	}
}

func TestStateEnvelopeCarriesProtocolState(t *testing.T) {
	env := runtime.StateEnvelope{
		FormatVersion: 1,
		Protocol:      "5.0",
		TypeName:      "talos_cluster",
		SchemaVersion: 3,
		Value:         runtime.DynamicValue{JSON: []byte(`{"id":"abc"}`)},
		Private:       []byte("provider-private"),
		Identity:      []byte("identity"),
	}

	if env.FormatVersion != 1 || env.SchemaVersion != 3 || len(env.Private) == 0 {
		t.Fatalf("state envelope lost required Terraform protocol state: %#v", env)
	}
}
