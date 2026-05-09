package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

var (
	providerNameRegex    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	exactTerraformSemver = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
)

// TerraformProviderIntegration represents an operator-configured Terraform
// provider that SuperPlane should load at startup.
type TerraformProviderIntegration struct {
	Name        string                  `json:"name"`
	Label       string                  `json:"label,omitempty"`
	Description string                  `json:"description,omitempty"`
	Icon        string                  `json:"icon,omitempty"`
	Source      string                  `json:"source"`
	Version     string                  `json:"version"`
	Expose      TerraformProviderExpose `json:"expose"`
}

// TerraformProviderExpose controls which resources and data sources are
// exposed as SuperPlane capabilities.
type TerraformProviderExpose struct {
	Resources          any `json:"resources"`          // "*" or []string
	DataSources        any `json:"dataSources"`        // "*" or []string
	Actions            any `json:"actions"`            // "*" or []string
	EphemeralResources any `json:"ephemeralResources"` // parsed, ignored
}

// TerraformProviderIntegrations parses the TERRAFORM_PROVIDER_INTEGRATIONS
// environment variable and returns a slice of validated provider configs.
// Returns nil, nil when the env var is empty.
func TerraformProviderIntegrations() ([]TerraformProviderIntegration, error) {
	raw := os.Getenv("TERRAFORM_PROVIDER_INTEGRATIONS")
	if raw == "" {
		return nil, nil
	}

	var providers []TerraformProviderIntegration
	if err := json.Unmarshal([]byte(raw), &providers); err != nil {
		return nil, fmt.Errorf("invalid TERRAFORM_PROVIDER_INTEGRATIONS JSON: %w", err)
	}

	seen := make(map[string]struct{}, len(providers))
	for i := range providers {
		p := &providers[i]

		if err := validateProviderName(p.Name); err != nil {
			return nil, fmt.Errorf("provider at index %d: %w", i, err)
		}

		if err := validateProviderSource(&p.Source); err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}
		if providerType(p.Source) != p.Name {
			return nil, fmt.Errorf("provider %q: name must match provider type %q", p.Name, providerType(p.Source))
		}

		if err := validateProviderVersion(p.Version); err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}

		if _, ok := seen[p.Name]; ok {
			return nil, fmt.Errorf("provider %q: duplicate name", p.Name)
		}
		seen[p.Name] = struct{}{}

		// Apply defaults
		if p.Label == "" {
			p.Label = p.Name
		}
		if p.Description == "" {
			p.Description = fmt.Sprintf("Terraform provider %s", p.Source)
		}
		if p.Expose.Resources == nil {
			p.Expose.Resources = []string{}
		}
		if p.Expose.DataSources == nil {
			p.Expose.DataSources = "*"
		}
		if p.Expose.Actions == nil {
			p.Expose.Actions = "*"
		}
		if err := validateExposeValue("resources", p.Expose.Resources); err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}
		if err := validateExposeValue("dataSources", p.Expose.DataSources); err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}
		if err := validateExposeValue("actions", p.Expose.Actions); err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}
	}

	return providers, nil
}

func validateProviderName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.Contains(name, ".") {
		return fmt.Errorf("name %q must not contain dots", name)
	}
	if !providerNameRegex.MatchString(name) {
		return fmt.Errorf("name %q must match %s", name, providerNameRegex.String())
	}
	return nil
}

func validateProviderSource(source *string) error {
	if *source == "" {
		return fmt.Errorf("source is required")
	}

	parts := strings.Split(*source, "/")
	if len(parts) == 2 {
		// <ns>/<type> -> default host
		*source = "registry.terraform.io/" + *source
		parts = strings.Split(*source, "/")
	}

	if len(parts) != 3 {
		return fmt.Errorf("source %q must match <host>/<namespace>/<type> or <namespace>/<type>", *source)
	}
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("source %q must not contain empty host, namespace, or type", *source)
		}
	}

	host := parts[0]
	if !registry.IsAllowedRegistryHost(host) {
		return fmt.Errorf("source host %q is not in the allowlist (registry.terraform.io)", host)
	}

	return nil
}

func validateExposeValue(name string, value any) error {
	switch v := value.(type) {
	case string:
		if v == "*" {
			return nil
		}
	case []any:
		for _, item := range v {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("expose.%s must contain only strings", name)
			}
		}
		return nil
	case []string:
		return nil
	}
	return fmt.Errorf("expose.%s must be \"*\" or an array of strings", name)
}

func providerType(source string) string {
	parts := strings.Split(source, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func validateProviderVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version is required")
	}
	if version == "latest" {
		return fmt.Errorf("version %q is not allowed; use an exact version", version)
	}
	// Reject constraint operators and separators commonly found in version constraints
	for _, ch := range []string{">", "<", "=", "~", "!", ",", " "} {
		if strings.Contains(version, ch) {
			return fmt.Errorf("version %q contains constraint operator %q; use an exact version", version, ch)
		}
	}
	if !exactTerraformSemver.MatchString(version) {
		return fmt.Errorf("version %q is not a valid exact SemVer", version)
	}
	return nil
}
