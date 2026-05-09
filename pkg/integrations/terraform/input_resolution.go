package terraform

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/superplanehq/superplane/pkg/core"
)

func resolveInputs(ctx core.ExecutionContext, inputs map[string]any) (map[string]any, error) {
	resolved := map[string]any{}
	for key, value := range inputs {
		v, err := resolveInputValue(ctx, value)
		if err != nil {
			return nil, err
		}
		resolved[key] = v
	}
	return resolved, nil
}

func resolveInputValue(ctx core.ExecutionContext, value any) (any, error) {
	switch v := value.(type) {
	case string:
		if strings.HasPrefix(v, "{{") && strings.HasSuffix(v, "}}") && ctx.Expressions != nil {
			expr := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(v, "{{"), "}}"))
			if strings.Contains(expr, "{{") || strings.Contains(expr, "}}") {
				return nil, fmt.Errorf("nested expressions not supported in terraform variable %q", v)
			}
			return ctx.Expressions.Run(expr)
		}
		return v, nil
	case map[string]any:
		if ref, ok := v["$terraformIntegrationSecret"].(map[string]any); ok {
			name, _ := ref["name"].(string)
			return resolveSecretRef(ctx.Integration, name)
		}
		out := map[string]any{}
		for key, child := range v {
			resolved, err := resolveInputValue(ctx, child)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			resolved, err := resolveInputValue(ctx, child)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return v, nil
	}
}

func resolveSecretRef(integration core.IntegrationContext, name string) (any, error) {
	if name == secretNameTerraformProviderConfig {
		return "", fmt.Errorf("terraform integration secret %q is reserved", name)
	}
	if !strings.HasPrefix(name, "terraform_output_") {
		return "", fmt.Errorf("terraform integration secret %q is not a trusted Terraform output secret", name)
	}
	if integration == nil {
		return "", fmt.Errorf("terraform integration is required to resolve secret %q", name)
	}
	secrets, err := integration.GetSecrets()
	if err != nil {
		return "", err
	}
	for _, secret := range secrets {
		if subtle.ConstantTimeCompare([]byte(secret.Name), []byte(name)) == 1 {
			var decoded any
			if err := json.Unmarshal(secret.Value, &decoded); err == nil {
				return decoded, nil
			}
			return string(secret.Value), nil
		}
	}
	return "", fmt.Errorf("terraform integration secret %q not found", name)
}
