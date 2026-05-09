package registry_test

import (
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

func TestChecksumForFilename(t *testing.T) {
	const checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sums := []byte(checksum + "  terraform-provider-talos_0.7.1_darwin_arm64.zip\n")

	got, err := registry.ChecksumForFilename(sums, "terraform-provider-talos_0.7.1_darwin_arm64.zip")
	if err != nil {
		t.Fatalf("ChecksumForFilename() error = %v", err)
	}
	if got != checksum {
		t.Errorf("got %q, want %q", got, checksum)
	}
}

func TestChecksumForFilenameRejectsUnsafeManifest(t *testing.T) {
	const checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tests := []struct {
		name     string
		sums     []byte
		filename string
	}{
		{
			name:     "filename missing",
			sums:     []byte(checksum + "  other.zip\n"),
			filename: "terraform-provider-talos_0.7.1_darwin_arm64.zip",
		},
		{
			name:     "duplicate filename entries",
			sums:     []byte(checksum + "  provider.zip\n" + checksum + "  provider.zip\n"),
			filename: "provider.zip",
		},
		{
			name:     "filename case mismatch",
			sums:     []byte(checksum + "  PROVIDER.zip\n"),
			filename: "provider.zip",
		},
		{
			name:     "path outside expected prefix",
			sums:     []byte(checksum + "  ../provider.zip\n"),
			filename: "../provider.zip",
		},
		{
			name:     "nested path",
			sums:     []byte(checksum + "  linux/provider.zip\n"),
			filename: "linux/provider.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := registry.ChecksumForFilename(tt.sums, tt.filename); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestVerifyPackageChecksumBinding(t *testing.T) {
	const checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sums := []byte(checksum + "  provider.zip\n")

	got, err := registry.VerifyPackageChecksumBinding(sums, "provider.zip", checksum)
	if err != nil {
		t.Fatalf("VerifyPackageChecksumBinding() error = %v", err)
	}
	if got != checksum {
		t.Errorf("got %q, want %q", got, checksum)
	}
}

func TestVerifyPackageChecksumBindingRejectsJSONMismatch(t *testing.T) {
	const checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const jsonChecksum = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	sums := []byte(checksum + "  provider.zip\n")

	if _, err := registry.VerifyPackageChecksumBinding(sums, "provider.zip", jsonChecksum); err == nil {
		t.Fatal("expected error")
	}
}
