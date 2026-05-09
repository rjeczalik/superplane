package toproto

import (
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/internal/tfpluginclient/tfprotov5/internal/tfplugin5"
)

func StringKind(in tfprotov5.StringKind) tfplugin5.StringKind {
	return tfplugin5.StringKind(in)
}
