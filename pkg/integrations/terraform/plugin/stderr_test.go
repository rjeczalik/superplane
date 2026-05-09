package plugin

import (
	"bytes"
	"strings"
	"testing"
)

func TestSanitizeStderr(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		want    string
		notWant string
	}{
		{
			name: "under limit passes through",
			raw:  []byte("provider warning"),
			want: "provider warning",
		},
		{
			name:    "over limit truncated",
			raw:     bytes.Repeat([]byte("x"), maxStderrBytes+100),
			want:    strings.Repeat("x", maxStderrBytes),
			notWant: strings.Repeat("x", maxStderrBytes+1),
		},
		{
			name:    "aws access key redacted",
			raw:     []byte("key AKIA1234567890ABCDEF leaked"),
			want:    "key [REDACTED] leaked",
			notWant: "AKIA1234567890ABCDEF",
		},
		{
			name:    "bearer token redacted",
			raw:     []byte("Authorization: Bearer secret.token-value_123"),
			want:    "Authorization: [REDACTED]",
			notWant: "secret.token-value_123",
		},
		{
			name:    "jwt credential redacted",
			raw:     []byte("token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abcdefghijklmnop.abcdefghijklmnop"),
			want:    "token [REDACTED]",
			notWant: "eyJhbGci",
		},
		{
			name:    "base64 json credential redacted",
			raw:     []byte("credential eyJ0eXBlIjoic2VydmljZV9hY2NvdW50IiwicHJvamVjdF9pZCI6InByb2QiLCJwcml2YXRlX2tleSI6InNlY3JldCJ9"),
			want:    "credential [REDACTED]",
			notWant: "c2VydmljZV9hY2NvdW50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(SanitizeStderr(tt.raw))
			if got != tt.want {
				t.Fatalf("SanitizeStderr() = %q, want %q", got, tt.want)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Fatalf("SanitizeStderr() leaked %q in %q", tt.notWant, got)
			}
		})
	}
}
