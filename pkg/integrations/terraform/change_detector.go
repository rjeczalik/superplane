package terraform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

func ComputeOutputsHash(outputs map[string]any, changedFields []string) (string, error) {
	projected := outputs
	if len(changedFields) > 0 {
		projected = make(map[string]any, len(changedFields))
		fields := append([]string(nil), changedFields...)
		sort.Strings(fields)
		for _, field := range fields {
			if value, ok := outputs[field]; ok {
				projected[field] = value
			}
		}
	}

	raw, err := json.Marshal(canonicalValue(projected))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]any, 0, len(keys))
		for _, key := range keys {
			out = append(out, []any{key, canonicalValue(typed[key])})
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = canonicalValue(item)
		}
		return out
	default:
		return value
	}
}
