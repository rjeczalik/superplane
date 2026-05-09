package toproto

import (
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov6/internal/tfplugin6"
)

func RawState(in *tfprotov6.RawState) *tfplugin6.RawState {
	if in == nil {
		return nil
	}

	resp := &tfplugin6.RawState{
		Json:    in.JSON,
		Flatmap: in.Flatmap,
	}

	return resp
}
