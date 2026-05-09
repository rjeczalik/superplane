package fromproto

import (
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov5/internal/tfplugin5"
)

func ResourceIdentityData(in *tfplugin5.ResourceIdentityData) *tfprotov5.ResourceIdentityData {
	if in == nil {
		return nil
	}

	resp := &tfprotov5.ResourceIdentityData{
		IdentityData: DynamicValue(in.IdentityData),
	}

	return resp
}
