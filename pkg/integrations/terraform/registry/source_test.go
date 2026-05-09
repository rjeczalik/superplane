package registry_test

import (
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

func TestParseProviderSource(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		host    string
		ns      string
		typ     string
	}{
		{name: "full source", input: "registry.terraform.io/siderolabs/talos", host: "registry.terraform.io", ns: "siderolabs", typ: "talos"},
		{name: "short source", input: "hashicorp/aws", host: "registry.terraform.io", ns: "hashicorp", typ: "aws"},
		{name: "disallowed host", input: "custom.registry.io/org/provider", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "single segment", input: "aws", wantErr: true},
		{name: "four segments", input: "a/b/c/d", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := registry.ParseProviderSource(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseProviderSource(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if src.Host() != tt.host || src.Namespace() != tt.ns || src.Type() != tt.typ {
				t.Errorf("got (%s, %s, %s), want (%s, %s, %s)", src.Host(), src.Namespace(), src.Type(), tt.host, tt.ns, tt.typ)
			}
		})
	}
}
