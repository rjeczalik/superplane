package fromproto

import (
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov5/internal/tfplugin5"
)

func ServerCapabilities(in *tfplugin5.ServerCapabilities) *tfprotov5.ServerCapabilities {
	if in == nil {
		return nil
	}

	return &tfprotov5.ServerCapabilities{
		GetProviderSchemaOptional: in.GetProviderSchemaOptional,
		MoveResourceState:         in.MoveResourceState,
		PlanDestroy:               in.PlanDestroy,
	}
}
