package fromproto

import (
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov5/internal/tfplugin5"
)

func StringKind(in tfplugin5.StringKind) tfprotov5.StringKind {
	return tfprotov5.StringKind(in)
}
