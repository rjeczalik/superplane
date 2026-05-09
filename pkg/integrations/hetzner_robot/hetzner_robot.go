package hetznerrobot

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/registry"
)

var _ core.Integration = (*HetznerRobot)(nil)

func init() {
	registry.RegisterIntegrationWithOptions("hetznerRobot", &HetznerRobot{}, registry.IntegrationRegistrationOptions{
		SetupProvider: &SetupProvider{},
	})
}

type HetznerRobot struct{}

type Configuration struct {
	Username string `json:"username" mapstructure:"username"`
	Password string `json:"password" mapstructure:"password"`
}

var validServerNumber = regexp.MustCompile(`^\d+$`)

func validateServerNumber(s string) error {
	if !validServerNumber.MatchString(s) {
		return fmt.Errorf("invalid server number %q: must be numeric", s)
	}
	return nil
}

var validFingerprint = regexp.MustCompile(`^[a-f0-9]{2}(:[a-f0-9]{2}){15}$`)

func validateFingerprint(s string) error {
	if !validFingerprint.MatchString(s) {
		return fmt.Errorf("invalid fingerprint %q: must be 16 colon-separated hex pairs", s)
	}
	return nil
}

func validatePortOrRange(s string) error {
	parts := strings.SplitN(s, "-", 2)
	for _, p := range parts {
		port, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return fmt.Errorf("invalid port %q", p)
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("port %d out of range (1-65535)", port)
		}
	}
	return nil
}

func validateCIDR(s string) error {
	if len(s) > 45 {
		return fmt.Errorf("CIDR too long (max 45 characters)")
	}
	_, _, err := net.ParseCIDR(s)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", s, err)
	}
	return nil
}

func (h *HetznerRobot) Name() string  { return "hetznerRobot" }
func (h *HetznerRobot) Label() string { return "Hetzner Robot" }
func (h *HetznerRobot) Icon() string  { return "hetzner_robot" }
func (h *HetznerRobot) Description() string {
	return "Manage Hetzner dedicated servers: rescue mode, SSH keys, Linux install, firewall, power, and naming"
}
func (h *HetznerRobot) Instructions() string {
	return `
**API Credentials:** Create a webservice user in your [Hetzner Robot panel](https://robot.your-server.de/) → Settings → Webservice and app settings. Use **Read & Write** access.
`
}
func (h *HetznerRobot) Configuration() []configuration.Field {
	return []configuration.Field{
		{Name: "username", Label: "API Username", Type: configuration.FieldTypeString, Required: true, Sensitive: false},
		{Name: "password", Label: "API Password", Type: configuration.FieldTypeString, Required: true, Sensitive: true},
	}
}
func (h *HetznerRobot) Actions() []core.Action {
	return []core.Action{
		&ListServers{},
		&GetServer{},
		&ResetServer{},
		&EnableRescue{},
		&DisableRescue{},
		&ListSSHKeys{},
		&AddSSHKey{},
		&DeleteSSHKey{},
		&InstallLinux{},
		&CancelLinuxInstall{},
		&ListFirewallRules{},
		&AddFirewallRule{},
		&UpdateFirewallRule{},
		&DeleteFirewallRule{},
		&WakeOnLAN{},
		&RenameServer{},
	}
}
func (h *HetznerRobot) Triggers() []core.Trigger                         { return nil }
func (h *HetznerRobot) Hooks() []core.Hook                               { return []core.Hook{} }
func (h *HetznerRobot) HandleHook(ctx core.IntegrationHookContext) error { return nil }
func (h *HetznerRobot) Cleanup(ctx core.IntegrationCleanupContext) error { return nil }
func (h *HetznerRobot) HandleRequest(ctx core.HTTPRequestContext)        {}

func (h *HetznerRobot) Sync(ctx core.SyncContext) error {
	config := Configuration{}
	if err := mapstructure.Decode(ctx.Configuration, &config); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	if strings.TrimSpace(config.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if strings.TrimSpace(config.Password) == "" {
		return fmt.Errorf("password is required")
	}
	client, err := NewClient(ctx.HTTP, ctx.Integration)
	if err != nil {
		return err
	}
	if err := client.Verify(); err != nil {
		return fmt.Errorf("failed to verify Hetzner Robot credentials: %w", err)
	}
	ctx.Integration.Ready()
	return nil
}

func (h *HetznerRobot) ListResources(resourceType string, ctx core.ListResourcesContext) ([]core.IntegrationResource, error) {
	switch resourceType {
	case "server":
		client, err := NewClient(ctx.HTTP, ctx.Integration)
		if err != nil {
			return nil, err
		}
		servers, err := client.ListServers()
		if err != nil {
			return nil, err
		}
		resources := make([]core.IntegrationResource, 0, len(servers))
		for _, s := range servers {
			name := s.ServerName
			if name == "" {
				name = fmt.Sprintf("Server %s", s.ServerNumber)
			}
			resources = append(resources, core.IntegrationResource{Type: "server", Name: name, ID: s.ServerNumber})
		}
		return resources, nil
	case "reset_type":
		return []core.IntegrationResource{
			{Type: "reset_type", Name: "Software Reset (CTRL+ALT+DEL)", ID: "sw"},
			{Type: "reset_type", Name: "Hardware Reset (power cycle)", ID: "hw"},
			{Type: "reset_type", Name: "Power button press", ID: "power"},
			{Type: "reset_type", Name: "Long power button press", ID: "power_long"},
			{Type: "reset_type", Name: "Manual reset (technician)", ID: "man"},
		}, nil
	case "ssh_key":
		client, err := NewClient(ctx.HTTP, ctx.Integration)
		if err != nil {
			return nil, err
		}
		keys, err := client.ListSSHKeys()
		if err != nil {
			return nil, err
		}
		resources := make([]core.IntegrationResource, 0, len(keys))
		for _, k := range keys {
			resources = append(resources, core.IntegrationResource{Type: "ssh_key", Name: k.Name, ID: k.Fingerprint})
		}
		return resources, nil
	default:
		return nil, nil
	}
}
