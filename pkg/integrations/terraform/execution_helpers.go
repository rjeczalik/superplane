package terraform

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/superplanehq/superplane/pkg/core"
)

func requireCapabilityEnabled(integration core.IntegrationContext, capability string) error {
	if accessor, ok := integration.(core.CapabilityStateAccessor); ok {
		for _, c := range accessor.Capabilities() {
			if c.Name != capability {
				continue
			}
			if c.State != core.IntegrationCapabilityStateEnabled {
				return fmt.Errorf("capability %q is not enabled", capability)
			}
			return nil
		}
		return fmt.Errorf("capability %q is not enabled", capability)
	}
	return nil
}

func loadExecutionProviderConfig(integration core.IntegrationContext) (map[string]any, error) {
	if integration == nil || integration.Secrets() == nil {
		return map[string]any{}, nil
	}
	raw, err := integration.Secrets().Get(secretNameTerraformProviderConfig)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if raw == "" {
		return map[string]any{}, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func mapConfig(value any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	if cfg, ok := value.(map[string]any); ok {
		return cfg, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (a *GeneratedAction) terraformType() string {
	return a.providerName + "_" + snakeCase(a.resourceName)
}

func snakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteString(strings.ToLower(string(r)))
	}
	return b.String()
}
