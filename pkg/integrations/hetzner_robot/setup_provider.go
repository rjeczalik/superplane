package hetznerrobot

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"

	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const (
	SetupStepCapabilitySelection = "capabilitySelection"
	SetupStepEnterCredentials    = "enterCredentials"
	SetupStepUpgradeCredentials  = "upgradeCredentials"
	SetupStepDone                = "done"

	SecretUsername = "username"
	SecretPassword = "password"

	PropertyCredentialPermission  = "credentialPermission"
	CredentialPermissionReadOnly  = "readOnly"
	CredentialPermissionReadWrite = "readWrite"
)

//go:embed templates/credentials-instructions.tpl
var credentialsInstructionsTemplate []byte

//go:embed templates/setup-complete.tpl
var setupCompleteTemplate []byte

// capabilityPermissions maps each capability name to the minimum credential
// permission level required to call the underlying Hetzner Robot API.
var capabilityPermissions = map[string]string{
	"hetznerRobot.listServers":        CredentialPermissionReadOnly,
	"hetznerRobot.getServer":          CredentialPermissionReadOnly,
	"hetznerRobot.renameServer":       CredentialPermissionReadWrite,
	"hetznerRobot.resetServer":        CredentialPermissionReadWrite,
	"hetznerRobot.wakeOnLan":          CredentialPermissionReadWrite,
	"hetznerRobot.listSshKeys":        CredentialPermissionReadOnly,
	"hetznerRobot.addSshKey":          CredentialPermissionReadWrite,
	"hetznerRobot.deleteSshKey":       CredentialPermissionReadWrite,
	"hetznerRobot.enableRescue":       CredentialPermissionReadWrite,
	"hetznerRobot.disableRescue":      CredentialPermissionReadWrite,
	"hetznerRobot.installLinux":       CredentialPermissionReadWrite,
	"hetznerRobot.cancelLinuxInstall": CredentialPermissionReadWrite,
	"hetznerRobot.listFirewallRules":  CredentialPermissionReadOnly,
	"hetznerRobot.addFirewallRule":    CredentialPermissionReadWrite,
	"hetznerRobot.updateFirewallRule": CredentialPermissionReadWrite,
	"hetznerRobot.deleteFirewallRule": CredentialPermissionReadWrite,
}

type SetupProvider struct{}

// ----- helpers -----

func genCapabilities(actions []core.Action) []core.Capability {
	caps := make([]core.Capability, 0, len(actions))
	for _, action := range actions {
		caps = append(caps, core.Capability{
			Type:           core.IntegrationCapabilityTypeAction,
			Name:           action.Name(),
			Label:          action.Label(),
			Description:    action.Description(),
			Configuration:  action.Configuration(),
			OutputChannels: action.OutputChannels(nil),
		})
	}
	return caps
}

func allCapabilityNames(groups []core.CapabilityGroup) []string {
	names := []string{}
	for _, group := range groups {
		for _, capability := range group.Capabilities {
			names = append(names, capability.Name)
		}
	}
	return names
}

func capabilityDiff(groups []core.CapabilityGroup, selected []string) []string {
	diff := []string{}
	for _, group := range groups {
		for _, capability := range group.Capabilities {
			if !slices.Contains(selected, capability.Name) {
				diff = append(diff, capability.Name)
			}
		}
	}
	return diff
}

func validateCapabilities(groups []core.CapabilityGroup, capabilities []string) error {
	known := map[string]struct{}{}
	for _, group := range groups {
		for _, capability := range group.Capabilities {
			known[capability.Name] = struct{}{}
		}
	}
	for _, name := range capabilities {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("unknown capability: %s", name)
		}
	}
	return nil
}

// requiredCredentialPermission returns the strongest permission level required
// by the given capabilities. It returns an error for any unknown capability —
// it never silently treats unknown names as read-only.
func requiredCredentialPermission(capabilities []string) (string, error) {
	required := CredentialPermissionReadOnly
	for _, name := range capabilities {
		perm, ok := capabilityPermissions[name]
		if !ok {
			return "", fmt.Errorf("unknown capability: %s", name)
		}
		if perm == CredentialPermissionReadWrite {
			required = CredentialPermissionReadWrite
		}
	}
	return required, nil
}

// credentialSatisfies reports whether a stored credential permission level
// satisfies a requested permission level.
func credentialSatisfies(stored, required string) bool {
	if stored == CredentialPermissionReadWrite {
		return true
	}
	if stored == CredentialPermissionReadOnly && required == CredentialPermissionReadOnly {
		return true
	}
	return false
}

func stringInput(inputs map[string]any, name string) (string, error) {
	raw, ok := inputs[name]
	if !ok {
		return "", fmt.Errorf("%s is required", name)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func renderTemplate(name string, body []byte, data any) (string, error) {
	tmpl, err := template.New(name).Parse(string(body))
	if err != nil {
		return "", fmt.Errorf("error parsing template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template %s: %w", name, err)
	}
	return buf.String(), nil
}

func inputsAsMap(value any) (map[string]any, error) {
	if value == nil {
		return nil, errors.New("invalid input")
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("invalid input")
	}
	return m, nil
}

// ----- interface methods -----

func (s *SetupProvider) CapabilityGroups() []core.CapabilityGroup {
	return []core.CapabilityGroup{
		{
			Label: "Server",
			Capabilities: genCapabilities([]core.Action{
				&ListServers{},
				&GetServer{},
				&RenameServer{},
				&ResetServer{},
				&WakeOnLAN{},
			}),
		},
		{
			Label: "SSH Keys",
			Capabilities: genCapabilities([]core.Action{
				&ListSSHKeys{},
				&AddSSHKey{},
				&DeleteSSHKey{},
			}),
		},
		{
			Label: "Rescue",
			Capabilities: genCapabilities([]core.Action{
				&EnableRescue{},
				&DisableRescue{},
			}),
		},
		{
			Label: "Linux Installation",
			Capabilities: genCapabilities([]core.Action{
				&InstallLinux{},
				&CancelLinuxInstall{},
			}),
		},
		{
			Label: "Firewall",
			Capabilities: genCapabilities([]core.Action{
				&ListFirewallRules{},
				&AddFirewallRule{},
				&UpdateFirewallRule{},
				&DeleteFirewallRule{},
			}),
		},
	}
}

func (s *SetupProvider) FirstStep(ctx core.SetupStepContext) core.SetupStep {
	return core.SetupStep{
		Type:         core.SetupStepTypeCapabilitySelection,
		Name:         SetupStepCapabilitySelection,
		Label:        "Select capabilities",
		Capabilities: allCapabilityNames(s.CapabilityGroups()),
	}
}

func (s *SetupProvider) OnStepSubmit(ctx core.SetupStepContext) (*core.SetupStep, error) {
	switch ctx.Step.Name {
	case SetupStepCapabilitySelection:
		return s.onCapabilitySelectionSubmit(ctx)
	case SetupStepEnterCredentials:
		return s.onEnterCredentialsSubmit(ctx)
	case SetupStepUpgradeCredentials:
		return s.onUpgradeCredentialsSubmit(ctx)
	}
	return nil, fmt.Errorf("unknown step: %s", ctx.Step.Name)
}

func (s *SetupProvider) onCapabilitySelectionSubmit(ctx core.SetupStepContext) (*core.SetupStep, error) {
	selected := ctx.Step.Capabilities
	if len(selected) == 0 {
		return nil, errors.New("at least one capability must be selected")
	}

	groups := s.CapabilityGroups()
	if err := validateCapabilities(groups, selected); err != nil {
		return nil, err
	}

	permission, err := requiredCredentialPermission(selected)
	if err != nil {
		return nil, err
	}

	// Move selected capabilities to the REQUESTED state, and the rest to
	// AVAILABLE — they may be requested later via OnCapabilityUpdate.
	ctx.Capabilities.Request(selected...)
	ctx.Capabilities.Available(capabilityDiff(groups, selected)...)

	instructions, err := renderTemplate("credentialsInstructions", credentialsInstructionsTemplate, map[string]any{
		"Permission": permission,
	})
	if err != nil {
		return nil, err
	}

	return &core.SetupStep{
		Type:         core.SetupStepTypeInputs,
		Name:         SetupStepEnterCredentials,
		Label:        "Enter Hetzner Robot credentials",
		Instructions: instructions,
		Inputs:       credentialFields(),
	}, nil
}

func credentialFields() []configuration.Field {
	return []configuration.Field{
		{
			Name:     SecretUsername,
			Label:    "Webservice username",
			Type:     configuration.FieldTypeString,
			Required: true,
		},
		{
			Name:      SecretPassword,
			Label:     "Webservice password",
			Type:      configuration.FieldTypeString,
			Required:  true,
			Sensitive: true,
		},
	}
}

func (s *SetupProvider) onEnterCredentialsSubmit(ctx core.SetupStepContext) (*core.SetupStep, error) {
	inputs, err := inputsAsMap(ctx.Step.Inputs)
	if err != nil {
		return nil, err
	}

	username, err := stringInput(inputs, SecretUsername)
	if err != nil {
		return nil, err
	}
	password, err := stringInput(inputs, SecretPassword)
	if err != nil {
		return nil, err
	}

	client, err := NewClientFromCredentials(ctx.HTTP, username, password)
	if err != nil {
		return nil, err
	}
	if err := client.Verify(); err != nil {
		return nil, fmt.Errorf("verify credentials: %w", err)
	}

	servers, err := client.ListServers()
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}

	requested := ctx.Capabilities.Requested()
	permission, err := requiredCredentialPermission(requested)
	if err != nil {
		return nil, err
	}

	if err := ctx.Secrets.CreateMany([]core.IntegrationSecretDefinition{
		{
			Name:        SecretUsername,
			Label:       "Webservice username",
			Description: "Hetzner Robot webservice username",
			Value:       username,
			Editable:    true,
		},
		{
			Name:        SecretPassword,
			Label:       "Webservice password",
			Description: "Hetzner Robot webservice password",
			Value:       password,
			Editable:    true,
		},
	}); err != nil {
		return nil, fmt.Errorf("error creating secrets: %w", err)
	}

	if err := ctx.Properties.Create(core.IntegrationPropertyDefinition{
		Name:        PropertyCredentialPermission,
		Label:       "Credential permission",
		Description: "Permission level of the stored Hetzner Robot webservice credentials",
		Type:        core.IntegrationPropertyTypeString,
		Value:       permission,
		Editable:    false,
	}); err != nil {
		return nil, fmt.Errorf("error creating property: %w", err)
	}

	ctx.Capabilities.Enable(requested...)

	instructions, err := renderTemplate("setupComplete", setupCompleteTemplate, map[string]any{
		"ServerCount": len(servers),
	})
	if err != nil {
		return nil, err
	}

	return &core.SetupStep{
		Type:         core.SetupStepTypeDone,
		Name:         SetupStepDone,
		Label:        "Setup complete",
		Instructions: instructions,
	}, nil
}

func (s *SetupProvider) onUpgradeCredentialsSubmit(ctx core.SetupStepContext) (*core.SetupStep, error) {
	inputs, err := inputsAsMap(ctx.Step.Inputs)
	if err != nil {
		return nil, err
	}

	username, err := stringInput(inputs, SecretUsername)
	if err != nil {
		return nil, err
	}
	password, err := stringInput(inputs, SecretPassword)
	if err != nil {
		return nil, err
	}

	client, err := NewClientFromCredentials(ctx.HTTP, username, password)
	if err != nil {
		return nil, err
	}
	if err := client.Verify(); err != nil {
		return nil, fmt.Errorf("verify credentials: %w", err)
	}

	servers, err := client.ListServers()
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}

	if err := ctx.Secrets.Update(SecretUsername, username); err != nil {
		return nil, fmt.Errorf("error updating username secret: %w", err)
	}
	if err := ctx.Secrets.Update(SecretPassword, password); err != nil {
		return nil, fmt.Errorf("error updating password secret: %w", err)
	}

	if err := s.upsertCredentialPermission(ctx.Properties, CredentialPermissionReadWrite); err != nil {
		return nil, err
	}

	ctx.Capabilities.Enable(ctx.Capabilities.Requested()...)

	instructions, err := renderTemplate("setupComplete", setupCompleteTemplate, map[string]any{
		"ServerCount": len(servers),
	})
	if err != nil {
		return nil, err
	}

	return &core.SetupStep{
		Type:         core.SetupStepTypeDone,
		Name:         SetupStepDone,
		Label:        "Setup complete",
		Instructions: instructions,
	}, nil
}

// upsertCredentialPermission writes the credential permission, replacing any
// existing value. Property storage's Create overwrites in the test fake, but
// production behaviour may differ — try Delete + Create to ensure replacement.
func (s *SetupProvider) upsertCredentialPermission(props core.IntegrationPropertyStorage, permission string) error {
	_ = props.Delete(PropertyCredentialPermission)
	return props.Create(core.IntegrationPropertyDefinition{
		Name:        PropertyCredentialPermission,
		Label:       "Credential permission",
		Description: "Permission level of the stored Hetzner Robot webservice credentials",
		Type:        core.IntegrationPropertyTypeString,
		Value:       permission,
		Editable:    false,
	})
}

func (s *SetupProvider) OnStepRevert(ctx core.SetupStepContext) error {
	switch ctx.Step.Name {
	case SetupStepCapabilitySelection:
		ctx.Capabilities.Clear()
		return nil
	case SetupStepEnterCredentials:
		if err := ctx.Secrets.Delete(SecretUsername); err != nil {
			return fmt.Errorf("error deleting username secret: %w", err)
		}
		if err := ctx.Secrets.Delete(SecretPassword); err != nil {
			return fmt.Errorf("error deleting password secret: %w", err)
		}
		if err := ctx.Properties.Delete(PropertyCredentialPermission); err != nil {
			return fmt.Errorf("error deleting credential permission property: %w", err)
		}
		return nil
	case SetupStepUpgradeCredentials:
		// Move requested capabilities back to available; preserve existing secrets
		// and already-enabled capabilities.
		requested := ctx.Capabilities.Requested()
		ctx.Capabilities.Available(requested...)
		return nil
	}
	return fmt.Errorf("unknown step: %s", ctx.Step.Name)
}

func (s *SetupProvider) OnPropertyUpdate(ctx core.PropertyUpdateContext) (*core.SetupStep, error) {
	return nil, fmt.Errorf("property updates are not supported for Hetzner Robot")
}

func (s *SetupProvider) OnSecretUpdate(ctx core.SecretUpdateContext) (*core.SetupStep, error) {
	if ctx.SecretName != SecretUsername && ctx.SecretName != SecretPassword {
		return nil, fmt.Errorf("unknown secret: %s", ctx.SecretName)
	}

	value, err := stringInput(map[string]any{ctx.SecretName: ctx.Value}, ctx.SecretName)
	if err != nil {
		return nil, err
	}

	storedPermission, _ := ctx.Properties.GetString(PropertyCredentialPermission)
	if storedPermission == CredentialPermissionReadWrite {
		// Both credentials must be rotated together when the stored
		// permission level is Read & Write — return an upgrade step.
		return &core.SetupStep{
			Type:         core.SetupStepTypeInputs,
			Name:         SetupStepUpgradeCredentials,
			Label:        "Re-enter Hetzner Robot credentials",
			Instructions: "Read & Write Hetzner Robot credentials must be rotated together. Please provide both the new username and password.",
			Inputs:       credentialFields(),
		}, nil
	}

	// Read-only stored permission: verify the new (mixed) pair, then update
	// the single secret being changed.
	otherName := SecretPassword
	if ctx.SecretName == SecretPassword {
		otherName = SecretUsername
	}
	other, err := ctx.Secrets.Get(otherName)
	if err != nil {
		return nil, fmt.Errorf("error reading existing %s secret: %w", otherName, err)
	}

	username, password := value, other
	if ctx.SecretName == SecretPassword {
		username, password = other, value
	}

	client, err := NewClientFromCredentials(ctx.HTTP, username, password)
	if err != nil {
		return nil, err
	}
	if err := client.Verify(); err != nil {
		return nil, fmt.Errorf("verify credentials: %w", err)
	}

	if err := ctx.Secrets.Update(ctx.SecretName, value); err != nil {
		return nil, fmt.Errorf("error updating %s secret: %w", ctx.SecretName, err)
	}
	return nil, nil
}

func (s *SetupProvider) OnCapabilityUpdate(ctx core.CapabilityUpdateContext) (*core.SetupStep, error) {
	requested, ok := ctx.Changes[core.IntegrationCapabilityStateRequested]
	if !ok || len(requested) == 0 {
		return nil, errors.New("no requested capabilities")
	}

	groups := s.CapabilityGroups()
	if err := validateCapabilities(groups, requested); err != nil {
		return nil, err
	}

	storedPermission, _ := ctx.Properties.GetString(PropertyCredentialPermission)
	required, err := requiredCredentialPermission(requested)
	if err != nil {
		return nil, err
	}

	if credentialSatisfies(storedPermission, required) {
		ctx.Capabilities.Enable(requested...)
		return nil, nil
	}

	ctx.Capabilities.Request(requested...)

	return &core.SetupStep{
		Type:         core.SetupStepTypeInputs,
		Name:         SetupStepUpgradeCredentials,
		Label:        "Upgrade Hetzner Robot credentials",
		Instructions: "The selected capabilities require Read & Write Hetzner Robot credentials. Please provide a webservice user with Read & Write access.",
		Inputs:       credentialFields(),
	}, nil
}

var _ core.IntegrationSetupProvider = (*SetupProvider)(nil)
