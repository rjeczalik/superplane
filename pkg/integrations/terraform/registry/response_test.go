package registry_test

import (
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

func TestParsePackageResponse(t *testing.T) {
	const baseURL = "https://registry.terraform.io/v1/providers/siderolabs/talos/0.7.1/download/darwin/arm64"

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{
			name: "valid response",
			raw: `{
				"protocols":["5.1","6.10"],
				"os":"darwin",
				"arch":"arm64",
				"filename":"terraform-provider-talos_0.7.1_darwin_arm64.zip",
				"download_url":"https://releases.hashicorp.com/provider.zip",
				"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
				"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
				"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}`,
		},
		{
			name: "unexpected fields rejected",
			raw: `{
				"protocols":["5.0"],
				"os":"darwin",
				"arch":"arm64",
				"filename":"provider.zip",
				"download_url":"https://releases.hashicorp.com/provider.zip",
				"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
				"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
				"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"extra":true
			}`,
			wantErr: true,
		},
		{
			name: "registry signing key metadata accepted",
			raw: `{
				"protocols":["5.0"],
				"os":"darwin",
				"arch":"arm64",
				"filename":"provider.zip",
				"download_url":"https://releases.hashicorp.com/provider.zip",
				"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
				"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
				"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"signing_keys":{
					"gpg_public_keys":[{
						"key_id":"34365D9472D7468F",
						"ascii_armor":"-----BEGIN PGP PUBLIC KEY BLOCK-----\n-----END PGP PUBLIC KEY BLOCK-----",
						"source":"HashiCorp",
						"source_url":"https://www.hashicorp.com/security",
						"trust_signature":"-----BEGIN PGP SIGNATURE-----\n-----END PGP SIGNATURE-----"
					}]
				}
			}`,
		},
		{
			name: "relative download URL resolves against registry response URL and is rejected",
			raw: `{
				"protocols":["5.0"],
				"os":"darwin",
				"arch":"arm64",
				"filename":"provider.zip",
				"download_url":"/provider.zip",
				"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
				"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
				"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}`,
			wantErr: true,
		},
		{
			name: "resolved non-HTTPS URL rejected",
			raw: `{
				"protocols":["5.0"],
				"os":"darwin",
				"arch":"arm64",
				"filename":"provider.zip",
				"download_url":"http://releases.hashicorp.com/provider.zip",
				"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
				"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
				"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}`,
			wantErr: true,
		},
		{
			name: "resolved host outside download allowlist rejected",
			raw: `{
				"protocols":["5.0"],
				"os":"darwin",
				"arch":"arm64",
				"filename":"provider.zip",
				"download_url":"https://example.com/provider.zip",
				"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
				"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
				"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}`,
			wantErr: true,
		},
		{
			name: "unsupported protocol major rejected",
			raw: `{
				"protocols":["4.0"],
				"os":"darwin",
				"arch":"arm64",
				"filename":"provider.zip",
				"download_url":"https://releases.hashicorp.com/provider.zip",
				"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
				"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
				"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := registry.ParsePackageResponse([]byte(tt.raw), baseURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePackageResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParsePackageResponseRejectsHTTPToHTTPSUpgrade(t *testing.T) {
	raw := []byte(`{
		"protocols":["6.10"],
		"os":"darwin",
		"arch":"arm64",
		"filename":"provider.zip",
		"download_url":"https://releases.hashicorp.com/provider.zip",
		"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
		"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
		"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}`)

	if _, err := registry.ParsePackageResponse(raw, "http://registry.terraform.io/v1/providers/x/y/z"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParsePackageResponseProtocolMinorCompatibility(t *testing.T) {
	raw := []byte(`{
		"protocols":["5.1"],
		"os":"darwin",
		"arch":"arm64",
		"filename":"provider.zip",
		"download_url":"https://releases.hashicorp.com/provider.zip",
		"shasums_url":"https://releases.hashicorp.com/provider_SHA256SUMS",
		"shasums_signature_url":"https://releases.hashicorp.com/provider_SHA256SUMS.sig",
		"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}`)

	response, err := registry.ParsePackageResponse(raw, "https://registry.terraform.io/v1/providers/x/y/z")
	if err != nil {
		t.Fatalf("ParsePackageResponse() error = %v", err)
	}
	if response.ProtocolMajor != 5 {
		t.Fatalf("ProtocolMajor = %d, want 5", response.ProtocolMajor)
	}
}
