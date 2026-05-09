package hetznerrobot

import (
	_ "embed"
	"sync"

	"github.com/superplanehq/superplane/pkg/utils"
)

//go:embed example_output_get_server.json
var exampleOutputGetServerBytes []byte

//go:embed example_output_reset_server.json
var exampleOutputResetServerBytes []byte

//go:embed example_output_enable_rescue.json
var exampleOutputEnableRescueBytes []byte

//go:embed example_output_disable_rescue.json
var exampleOutputDisableRescueBytes []byte

//go:embed example_output_wake_on_lan.json
var exampleOutputWakeOnLANBytes []byte

//go:embed example_output_rename_server.json
var exampleOutputRenameServerBytes []byte

//go:embed example_output_list_ssh_keys.json
var exampleOutputListSSHKeysBytes []byte

//go:embed example_output_add_ssh_key.json
var exampleOutputAddSSHKeyBytes []byte

//go:embed example_output_delete_ssh_key.json
var exampleOutputDeleteSSHKeyBytes []byte

//go:embed example_output_linux_install.json
var exampleOutputLinuxInstallBytes []byte

//go:embed example_output_cancel_linux_install.json
var exampleOutputCancelLinuxInstallBytes []byte

//go:embed example_output_list_firewall_rules.json
var exampleOutputListFirewallRulesBytes []byte

//go:embed example_output_add_firewall_rule.json
var exampleOutputAddFirewallRuleBytes []byte

//go:embed example_output_update_firewall_rule.json
var exampleOutputUpdateFirewallRuleBytes []byte

//go:embed example_output_delete_firewall_rule.json
var exampleOutputDeleteFirewallRuleBytes []byte

//go:embed example_output_list_servers.json
var exampleOutputListServersBytes []byte

var (
	exampleOutputGetServerOnce sync.Once
	exampleOutputGetServer     map[string]any

	exampleOutputResetServerOnce sync.Once
	exampleOutputResetServer     map[string]any

	exampleOutputEnableRescueOnce sync.Once
	exampleOutputEnableRescue     map[string]any

	exampleOutputDisableRescueOnce sync.Once
	exampleOutputDisableRescue     map[string]any

	exampleOutputWakeOnLANOnce sync.Once
	exampleOutputWakeOnLAN     map[string]any

	exampleOutputRenameServerOnce sync.Once
	exampleOutputRenameServer     map[string]any

	exampleOutputListSSHKeysOnce sync.Once
	exampleOutputListSSHKeys     map[string]any

	exampleOutputAddSSHKeyOnce sync.Once
	exampleOutputAddSSHKey     map[string]any

	exampleOutputDeleteSSHKeyOnce sync.Once
	exampleOutputDeleteSSHKey     map[string]any

	exampleOutputLinuxInstallOnce sync.Once
	exampleOutputLinuxInstall     map[string]any

	exampleOutputCancelLinuxInstallOnce sync.Once
	exampleOutputCancelLinuxInstall     map[string]any

	exampleOutputListFirewallRulesOnce sync.Once
	exampleOutputListFirewallRules     map[string]any

	exampleOutputAddFirewallRuleOnce sync.Once
	exampleOutputAddFirewallRule     map[string]any

	exampleOutputUpdateFirewallRuleOnce sync.Once
	exampleOutputUpdateFirewallRule     map[string]any

	exampleOutputDeleteFirewallRuleOnce sync.Once
	exampleOutputDeleteFirewallRule     map[string]any

	exampleOutputListServersOnce sync.Once
	exampleOutputListServers     map[string]any
)

func (g *GetServer) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputGetServerOnce, exampleOutputGetServerBytes, &exampleOutputGetServer)
}

func (r *ResetServer) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputResetServerOnce, exampleOutputResetServerBytes, &exampleOutputResetServer)
}

func (e *EnableRescue) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputEnableRescueOnce, exampleOutputEnableRescueBytes, &exampleOutputEnableRescue)
}

func (d *DisableRescue) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputDisableRescueOnce, exampleOutputDisableRescueBytes, &exampleOutputDisableRescue)
}

func (w *WakeOnLAN) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputWakeOnLANOnce, exampleOutputWakeOnLANBytes, &exampleOutputWakeOnLAN)
}

func (r *RenameServer) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputRenameServerOnce, exampleOutputRenameServerBytes, &exampleOutputRenameServer)
}

func (l *ListSSHKeys) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputListSSHKeysOnce, exampleOutputListSSHKeysBytes, &exampleOutputListSSHKeys)
}

func (a *AddSSHKey) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputAddSSHKeyOnce, exampleOutputAddSSHKeyBytes, &exampleOutputAddSSHKey)
}

func (d *DeleteSSHKey) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputDeleteSSHKeyOnce, exampleOutputDeleteSSHKeyBytes, &exampleOutputDeleteSSHKey)
}

func (i *InstallLinux) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputLinuxInstallOnce, exampleOutputLinuxInstallBytes, &exampleOutputLinuxInstall)
}

func (c *CancelLinuxInstall) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputCancelLinuxInstallOnce, exampleOutputCancelLinuxInstallBytes, &exampleOutputCancelLinuxInstall)
}

func (l *ListFirewallRules) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputListFirewallRulesOnce, exampleOutputListFirewallRulesBytes, &exampleOutputListFirewallRules)
}

func (a *AddFirewallRule) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputAddFirewallRuleOnce, exampleOutputAddFirewallRuleBytes, &exampleOutputAddFirewallRule)
}

func (u *UpdateFirewallRule) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputUpdateFirewallRuleOnce, exampleOutputUpdateFirewallRuleBytes, &exampleOutputUpdateFirewallRule)
}

func (d *DeleteFirewallRule) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputDeleteFirewallRuleOnce, exampleOutputDeleteFirewallRuleBytes, &exampleOutputDeleteFirewallRule)
}

func (l *ListServers) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputListServersOnce, exampleOutputListServersBytes, &exampleOutputListServers)
}
