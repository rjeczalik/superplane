package registry_test

import (
	"strings"
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

func TestVerifyChecksum(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedHex string
		wantErr     bool
	}{
		{
			name:        "matching hash",
			input:       "terraform-provider",
			expectedHex: "097f18785a3b9ecf1bf732fbf142b004b2e90e2bb5e587d7ac3126b3424a1f4e",
		},
		{
			name:        "mismatched hash",
			input:       "terraform-provider",
			expectedHex: "0000000000000000000000000000000000000000000000000000000000000000",
			wantErr:     true,
		},
		{
			name:        "truncated input",
			input:       "terraform",
			expectedHex: "e8d2c69e2dac9d80df0523ebbfe260da49bfb8d849b7b8d2fa098b71a9140d61",
			wantErr:     true,
		},
		{
			name:        "empty input",
			input:       "",
			expectedHex: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:        "invalid hex",
			input:       "terraform-provider",
			expectedHex: "not-hex",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.VerifyChecksum(strings.NewReader(tt.input), tt.expectedHex)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifyChecksum() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
