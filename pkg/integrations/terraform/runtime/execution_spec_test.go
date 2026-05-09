package runtime_test

import (
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestExecutionSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    runtime.ExecutionSpec
		wantErr bool
	}{
		{
			name: "valid",
			spec: runtime.ExecutionSpec{
				CapabilityName:  "talos.cluster.apply",
				ProviderName:    "talos",
				ProviderSource:  "registry.terraform.io/siderolabs/talos",
				ProviderVersion: "0.7.1",
				ResourceName:    "talos_cluster",
				Operation:       runtime.OpApply,
				SchemaHash:      "abc123",
			},
		},
		{
			name: "missing capability",
			spec: runtime.ExecutionSpec{
				ProviderName:    "x",
				ProviderSource:  "y",
				ProviderVersion: "1.0.0",
				ResourceName:    "r",
				Operation:       runtime.OpApply,
			},
			wantErr: true,
		},
		{
			name: "missing provider name",
			spec: runtime.ExecutionSpec{
				CapabilityName:  "x",
				ProviderSource:  "y",
				ProviderVersion: "1.0.0",
				ResourceName:    "r",
				Operation:       runtime.OpApply,
			},
			wantErr: true,
		},
		{
			name: "missing operation",
			spec: runtime.ExecutionSpec{
				CapabilityName:  "x",
				ProviderName:    "p",
				ProviderSource:  "y",
				ProviderVersion: "1.0.0",
				ResourceName:    "r",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
