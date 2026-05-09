package v5

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func DiagnosticsFromTFProto(diags []*tfprotov5.Diagnostic) []runtime.ProviderDiagnostic {
	if len(diags) == 0 {
		return nil
	}

	out := make([]runtime.ProviderDiagnostic, 0, len(diags))
	for _, diag := range diags {
		if diag == nil {
			continue
		}
		out = append(out, DiagnosticFromTFProto(diag))
	}
	return out
}

func DiagnosticFromTFProto(diag *tfprotov5.Diagnostic) runtime.ProviderDiagnostic {
	if diag == nil {
		return runtime.ProviderDiagnostic{}
	}

	out := runtime.ProviderDiagnostic{
		Severity: severityFromTFProto(diag.Severity),
		Summary:  diag.Summary,
		Detail:   diag.Detail,
	}
	if diag.Attribute != nil {
		out.Attribute = formatAttributePath(diag.Attribute)
	}

	return out
}

func formatAttributePath(path *tftypes.AttributePath) string {
	if path == nil {
		return ""
	}

	var out strings.Builder
	for _, step := range path.Steps() {
		switch s := step.(type) {
		case tftypes.AttributeName:
			if out.Len() > 0 {
				out.WriteString(".")
			}
			out.WriteString(string(s))
		case tftypes.ElementKeyInt:
			out.WriteString("[")
			out.WriteString(strconv.FormatInt(int64(s), 10))
			out.WriteString("]")
		case tftypes.ElementKeyString:
			out.WriteString("[")
			out.WriteString(strconv.Quote(string(s)))
			out.WriteString("]")
		default:
			if out.Len() > 0 {
				out.WriteString(".")
			}
			out.WriteString(fmt.Sprint(s))
		}
	}

	return out.String()
}

func severityFromTFProto(severity tfprotov5.DiagnosticSeverity) runtime.DiagSeverity {
	switch severity {
	case tfprotov5.DiagnosticSeverityWarning:
		return runtime.DiagWarning
	default:
		return runtime.DiagError
	}
}
