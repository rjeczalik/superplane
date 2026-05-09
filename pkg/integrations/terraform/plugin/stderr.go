package plugin

import "regexp"

const maxStderrBytes = 4 << 10

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9+/=_-]{40,}`),
}

func SanitizeStderr(raw []byte) []byte {
	if len(raw) > maxStderrBytes {
		raw = raw[:maxStderrBytes]
	}
	sanitized := string(raw)
	for _, pattern := range credentialPatterns {
		sanitized = pattern.ReplaceAllString(sanitized, "[REDACTED]")
	}
	return []byte(sanitized)
}
