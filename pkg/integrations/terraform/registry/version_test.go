package registry_test

import (
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

func TestParseProviderVersion(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{input: "0.7.1"},
		{input: "1.0.0-beta1"},
		{input: "0.7", wantErr: true},
		{input: "v0.7.1", wantErr: true},
		{input: "latest", wantErr: true},
		{input: ">= 1.0.0", wantErr: true},
		{input: "~> 0.7", wantErr: true},
		{input: "1.0.0, 2.0.0", wantErr: true},
		{input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			version, err := registry.ParseProviderVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseProviderVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil && version.String() != tt.input {
				t.Errorf("got %q, want %q", version.String(), tt.input)
			}
		})
	}
}
