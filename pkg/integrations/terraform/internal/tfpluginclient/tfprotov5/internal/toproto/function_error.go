package toproto

import (
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov5/internal/tfplugin5"
)

func FunctionError(in *tfprotov5.FunctionError) *tfplugin5.FunctionError {
	if in == nil {
		return nil
	}

	resp := &tfplugin5.FunctionError{
		FunctionArgument: in.FunctionArgument,
		Text:             ForceValidUTF8(in.Text),
	}

	return resp
}
