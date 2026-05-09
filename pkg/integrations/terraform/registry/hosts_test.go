package registry_test

import (
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

func TestHostAllowlists(t *testing.T) {
	tests := []struct {
		name string
		host string
		fn   func(string) bool
		want bool
	}{
		{name: "registry host allowed", host: "registry.terraform.io", fn: registry.IsAllowedRegistryHost, want: true},
		{name: "registry host rejected", host: "example.com", fn: registry.IsAllowedRegistryHost},
		{name: "hashicorp releases download host allowed", host: "releases.hashicorp.com", fn: registry.IsAllowedDownloadHost, want: true},
		{name: "github download host allowed", host: "github.com", fn: registry.IsAllowedDownloadHost, want: true},
		{name: "github release assets download host allowed", host: "release-assets.githubusercontent.com", fn: registry.IsAllowedDownloadHost, want: true},
		{name: "download host rejected", host: "registry.terraform.io", fn: registry.IsAllowedDownloadHost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(tt.host); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
