package toproto

import (
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov5/internal/tfplugin5"
)

func ServerCapabilities(in *tfprotov5.ServerCapabilities) *tfplugin5.ServerCapabilities {
	if in == nil {
		return nil
	}

	resp := &tfplugin5.ServerCapabilities{
		GetProviderSchemaOptional: in.GetProviderSchemaOptional,
		MoveResourceState:         in.MoveResourceState,
		PlanDestroy:               in.PlanDestroy,
	}

	return resp
}
