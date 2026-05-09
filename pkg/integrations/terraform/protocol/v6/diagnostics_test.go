package v6

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestDiagnosticFromTFProto_Error(t *testing.T) {
	got := DiagnosticFromTFProto(&tfprotov6.Diagnostic{
		Severity: tfprotov6.DiagnosticSeverityError,
		Summary:  "invalid config",
		Detail:   "name is required",
	})

	assert.Equal(t, runtime.ProviderDiagnostic{
		Severity: runtime.DiagError,
		Summary:  "invalid config",
		Detail:   "name is required",
	}, got)
}

func TestDiagnosticFromTFProto_Warning(t *testing.T) {
	got := DiagnosticFromTFProto(&tfprotov6.Diagnostic{
		Severity: tfprotov6.DiagnosticSeverityWarning,
		Summary:  "deprecated field",
		Detail:   "use replacement instead",
	})

	assert.Equal(t, runtime.ProviderDiagnostic{
		Severity: runtime.DiagWarning,
		Summary:  "deprecated field",
		Detail:   "use replacement instead",
	}, got)
}

func TestDiagnosticFromTFProto_WithAttributePath(t *testing.T) {
	path := tftypes.NewAttributePath().
		WithAttributeName("config").
		WithAttributeName("rules").
		WithElementKeyInt(0).
		WithAttributeName("name")

	got := DiagnosticFromTFProto(&tfprotov6.Diagnostic{
		Severity:  tfprotov6.DiagnosticSeverityError,
		Summary:   "invalid name",
		Attribute: path,
	})

	assert.Equal(t, runtime.DiagError, got.Severity)
	assert.Equal(t, "invalid name", got.Summary)
	assert.Equal(t, `config.rules[0].name`, got.Attribute)
}

func TestDiagnosticsFromTFProto_SkipsNilDiagnostics(t *testing.T) {
	got := DiagnosticsFromTFProto([]*tfprotov6.Diagnostic{
		nil,
		{Severity: tfprotov6.DiagnosticSeverityWarning, Summary: "warn"},
	})

	assert.Equal(t, []runtime.ProviderDiagnostic{
		{Severity: runtime.DiagWarning, Summary: "warn"},
	}, got)
}

func TestDiagnosticFromTFProto_InvalidSeverityDefaultsToError(t *testing.T) {
	got := DiagnosticFromTFProto(&tfprotov6.Diagnostic{
		Severity: tfprotov6.DiagnosticSeverityInvalid,
		Summary:  "invalid severity",
	})

	assert.Equal(t, runtime.DiagError, got.Severity)
}
