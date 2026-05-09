package terraform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/superplanehq/superplane/pkg/core"
)

var secretPathUnsafe = regexp.MustCompile(`\.\.|/|\x00`)
var secretNameChar = regexp.MustCompile(`[^A-Za-z0-9_]`)

type TerraformSecretWriter interface {
	SetSecret(name string, value []byte) error
}

func filterSensitiveOutputs(ctx core.ExecutionContext, payload map[string]any, sensitive map[string]struct{}, canvasID string, nodeID string) error {
	sanitized, _, err := SanitizeTerraformOutputs(canvasID, nodeID, "", "", ctx.Integration, payload, sensitive)
	if err != nil {
		return err
	}
	for key := range payload {
		delete(payload, key)
	}
	for key, value := range sanitized {
		payload[key] = value
	}
	return nil
}

func SanitizeTerraformOutputs(
	canvasID string,
	nodeID string,
	managedResourceID string,
	operationID string,
	secretWriter TerraformSecretWriter,
	payload map[string]any,
	sensitive map[string]struct{},
) (map[string]any, map[string]any, error) {
	sanitized := cloneMap(payload)
	hashInput := cloneMap(payload)

	for path := range sensitive {
		if secretPathUnsafe.MatchString(path) {
			return nil, nil, fmt.Errorf("invalid sensitive output path %q", path)
		}
		value, ok := valueAtPath(sanitized, path)
		replacePath := path
		if !ok {
			root := strings.Split(path, ".")[0]
			value, ok = sanitized[root]
			if !ok {
				continue
			}
			replacePath = root
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		name := terraformOutputSecretName(canvasID, nodeID, managedResourceID, operationID, path)
		if err := secretWriter.SetSecret(name, raw); err != nil {
			return nil, nil, err
		}
		setValueAtPath(sanitized, replacePath, map[string]any{"$terraformIntegrationSecret": map[string]any{"name": name}})
		setValueAtPath(hashInput, replacePath, "__terraform_sensitive__")
	}
	return sanitized, hashInput, nil
}

func RedactTerraformDiagnostics(diagnostics any, sensitiveAttrs map[string]struct{}, sensitiveConfigValues []string) any {
	raw, err := json.Marshal(diagnostics)
	if err != nil {
		return diagnostics
	}
	redacted := string(raw)
	for value := range sensitiveAttrs {
		if value != "" {
			redacted = strings.ReplaceAll(redacted, value, "[REDACTED]")
		}
	}
	for _, value := range sensitiveConfigValues {
		if value != "" {
			redacted = strings.ReplaceAll(redacted, value, "[REDACTED]")
		}
	}
	var out any
	if err := json.NewDecoder(bytes.NewReader([]byte(redacted))).Decode(&out); err != nil {
		return redacted
	}
	return out
}

func terraformOutputSecretName(canvasID, nodeID, managedResourceID, operationID, path string) string {
	parts := []string{"terraform_output", canvasID, nodeID}
	if managedResourceID != "" {
		parts = append(parts, managedResourceID)
	}
	if operationID != "" {
		parts = append(parts, operationID)
	}
	parts = append(parts, path)
	return secretNameChar.ReplaceAllString(strings.Join(parts, "_"), "_")
}

func cloneMap(payload map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range payload {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for i, item := range typed {
			out[i] = cloneMap(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneAny(item)
		}
		return out
	default:
		return typed
	}
}

func valueAtPath(payload map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	current := any(payload)
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func setValueAtPath(payload map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := payload
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			return
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}
