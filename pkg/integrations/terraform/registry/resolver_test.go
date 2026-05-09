package registry_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

func TestRegistryResolverResolveProvider(t *testing.T) {
	entity := newTestEntity(t)
	archive := []byte("provider archive")
	checksum := sha256Hex(archive)
	filename := "terraform-provider-talos_0.11.0_darwin_arm64.zip"
	sums := []byte(fmt.Sprintf("%s  %s\n", checksum, filename))
	signature := detachedSignature(t, entity, sums)
	requests := map[string][]byte{
		"/v1/providers/siderolabs/talos/0.11.0/download/darwin/arm64": []byte(fmt.Sprintf(`{
			"protocols":["5.0","6.0"],
			"os":"darwin",
			"arch":"arm64",
			"filename":%q,
			"download_url":"https://github.com/siderolabs/terraform-provider-talos/releases/download/v0.11.0/%s",
			"shasums_url":"https://github.com/siderolabs/terraform-provider-talos/releases/download/v0.11.0/terraform-provider-talos_0.11.0_SHA256SUMS",
			"shasums_signature_url":"https://github.com/siderolabs/terraform-provider-talos/releases/download/v0.11.0/terraform-provider-talos_0.11.0_SHA256SUMS.sig",
			"shasum":%q,
			"signing_keys":{"gpg_public_keys":[{"key_id":"test","ascii_armor":%q}]}
		}`, filename, filename, checksum, armoredEntity(t, entity))),
		"/siderolabs/terraform-provider-talos/releases/download/v0.11.0/terraform-provider-talos_0.11.0_SHA256SUMS":     sums,
		"/siderolabs/terraform-provider-talos/releases/download/v0.11.0/terraform-provider-talos_0.11.0_SHA256SUMS.sig": signature,
		"/siderolabs/terraform-provider-talos/releases/download/v0.11.0/" + filename:                                    archive,
	}

	resolver := registry.NewRegistryResolver(
		registry.WithHTTPClient(testRegistryClient(t, requests)),
		registry.WithRegistrySigningKeysForTests(),
		registry.WithPlatform("darwin", "arm64"),
	)
	source, err := registry.ParseProviderSource("siderolabs/talos")
	if err != nil {
		t.Fatal(err)
	}
	version, err := registry.ParseProviderVersion("0.11.0")
	if err != nil {
		t.Fatal(err)
	}

	pkg, err := resolver.ResolveProvider(context.Background(), source, version)
	if err != nil {
		t.Fatalf("ResolveProvider() error = %v", err)
	}

	if pkg.ProtocolMajor != 6 {
		t.Fatalf("ProtocolMajor = %d, want 6", pkg.ProtocolMajor)
	}
	if pkg.SHA256 != checksum {
		t.Fatalf("SHA256 = %q, want %q", pkg.SHA256, checksum)
	}
	if !bytes.Equal(pkg.Archive, archive) {
		t.Fatalf("Archive = %q, want %q", pkg.Archive, archive)
	}
	if pkg.Platform != "darwin_arm64" {
		t.Fatalf("Platform = %q, want darwin_arm64", pkg.Platform)
	}
}

func TestRegistryResolverRejectsRegistrySigningKeysByDefault(t *testing.T) {
	entity := newTestEntity(t)
	archive := []byte("provider archive")
	checksum := sha256Hex(archive)
	filename := "terraform-provider-talos_0.11.0_darwin_arm64.zip"
	sums := []byte(fmt.Sprintf("%s  %s\n", checksum, filename))
	signature := detachedSignature(t, entity, sums)
	requests := map[string][]byte{
		"/v1/providers/siderolabs/talos/0.11.0/download/darwin/arm64": []byte(fmt.Sprintf(`{
			"protocols":["6.0"],
			"os":"darwin",
			"arch":"arm64",
			"filename":%q,
			"download_url":"https://github.com/provider.zip",
			"shasums_url":"https://github.com/provider_SHA256SUMS",
			"shasums_signature_url":"https://github.com/provider_SHA256SUMS.sig",
			"shasum":%q,
			"signing_keys":{"gpg_public_keys":[{"key_id":"test","ascii_armor":%q}]}
		}`, filename, checksum, armoredEntity(t, entity))),
		"/provider_SHA256SUMS":     sums,
		"/provider_SHA256SUMS.sig": signature,
	}

	resolver := registry.NewRegistryResolver(
		registry.WithHTTPClient(testRegistryClient(t, requests)),
		registry.WithPlatform("darwin", "arm64"),
	)
	source, _ := registry.ParseProviderSource("siderolabs/talos")
	version, _ := registry.ParseProviderVersion("0.11.0")

	err := expectResolveProviderError(resolver, source, version)
	if !strings.Contains(err.Error(), "registry-provided signing keys are not trusted") {
		t.Fatalf("error = %v", err)
	}
}

func TestRegistryResolverUsesKeyPinStoreTOFU(t *testing.T) {
	entity := newTestEntity(t)
	archive := []byte("provider archive")
	checksum := sha256Hex(archive)
	filename := "terraform-provider-talos_0.11.0_darwin_arm64.zip"
	sums := []byte(fmt.Sprintf("%s  %s\n", checksum, filename))
	signature := detachedSignature(t, entity, sums)
	requests := map[string][]byte{
		"/v1/providers/siderolabs/talos/0.11.0/download/darwin/arm64": []byte(fmt.Sprintf(`{
			"protocols":["6.0"],
			"os":"darwin",
			"arch":"arm64",
			"filename":%q,
			"download_url":"https://github.com/provider.zip",
			"shasums_url":"https://github.com/provider_SHA256SUMS",
			"shasums_signature_url":"https://github.com/provider_SHA256SUMS.sig",
			"shasum":%q,
			"signing_keys":{"gpg_public_keys":[{"key_id":"test","ascii_armor":%q}]}
		}`, filename, checksum, armoredEntity(t, entity))),
		"/provider_SHA256SUMS":     sums,
		"/provider_SHA256SUMS.sig": signature,
		"/provider.zip":            archive,
	}

	resolver := registry.NewRegistryResolver(
		registry.WithHTTPClient(testRegistryClient(t, requests)),
		registry.WithKeyPinStore(newKeyPinStore(t), registry.KeyPinPolicy{AllowTOFU: true}),
		registry.WithPlatform("darwin", "arm64"),
	)
	source, _ := registry.ParseProviderSource("siderolabs/talos")
	version, _ := registry.ParseProviderVersion("0.11.0")

	pkg, err := resolver.ResolveProvider(context.Background(), source, version)
	if err != nil {
		t.Fatalf("ResolveProvider() error = %v", err)
	}
	if !bytes.Equal(pkg.Archive, archive) {
		t.Fatalf("Archive = %q, want %q", pkg.Archive, archive)
	}
}

func TestRegistryResolverCanSkipSignatureVerificationForDevelopment(t *testing.T) {
	archive := []byte("provider archive")
	checksum := sha256Hex(archive)
	filename := "terraform-provider-talos_0.11.0_darwin_arm64.zip"
	sums := []byte(fmt.Sprintf("%s  %s\n", checksum, filename))
	requests := map[string][]byte{
		"/v1/providers/siderolabs/talos/0.11.0/download/darwin/arm64": []byte(fmt.Sprintf(`{
			"protocols":["6.0"],
			"os":"darwin",
			"arch":"arm64",
			"filename":%q,
			"download_url":"https://github.com/provider.zip",
			"shasums_url":"https://github.com/provider_SHA256SUMS",
			"shasums_signature_url":"https://github.com/provider_SHA256SUMS.sig",
			"shasum":%q
		}`, filename, checksum)),
		"/provider_SHA256SUMS":     sums,
		"/provider_SHA256SUMS.sig": []byte("not a valid signature"),
		"/provider.zip":            archive,
	}

	resolver := registry.NewRegistryResolver(
		registry.WithHTTPClient(testRegistryClient(t, requests)),
		registry.WithSignatureVerificationSkipped(),
		registry.WithPlatform("darwin", "arm64"),
	)
	source, _ := registry.ParseProviderSource("siderolabs/talos")
	version, _ := registry.ParseProviderVersion("0.11.0")

	pkg, err := resolver.ResolveProvider(context.Background(), source, version)
	if err != nil {
		t.Fatalf("ResolveProvider() error = %v", err)
	}
	if !bytes.Equal(pkg.Archive, archive) {
		t.Fatalf("Archive = %q, want %q", pkg.Archive, archive)
	}
}

func TestRegistryResolverRejectsUnsupportedProtocols(t *testing.T) {
	requests := map[string][]byte{
		"/v1/providers/siderolabs/talos/0.11.0/download/darwin/arm64": []byte(`{
			"protocols":["4.0"],
			"os":"darwin",
			"arch":"arm64",
			"filename":"terraform-provider-talos_0.11.0_darwin_arm64.zip",
			"download_url":"https://github.com/provider.zip",
			"shasums_url":"https://github.com/provider_SHA256SUMS",
			"shasums_signature_url":"https://github.com/provider_SHA256SUMS.sig",
			"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}`),
	}

	resolver := registry.NewRegistryResolver(
		registry.WithHTTPClient(testRegistryClient(t, requests)),
		registry.WithRegistrySigningKeysForTests(),
		registry.WithPlatform("darwin", "arm64"),
	)
	source, _ := registry.ParseProviderSource("siderolabs/talos")
	version, _ := registry.ParseProviderVersion("0.11.0")

	err := expectResolveProviderError(resolver, source, version)
	if !strings.Contains(err.Error(), "does not advertise Terraform protocol v5 or v6") {
		t.Fatalf("error = %v", err)
	}
}

func TestRegistryResolverRejectsChecksumMismatch(t *testing.T) {
	entity := newTestEntity(t)
	archive := []byte("provider archive")
	filename := "terraform-provider-talos_0.11.0_darwin_arm64.zip"
	sums := []byte(fmt.Sprintf("%s  %s\n", sha256Hex(archive), filename))
	signature := detachedSignature(t, entity, sums)
	requests := map[string][]byte{
		"/v1/providers/siderolabs/talos/0.11.0/download/darwin/arm64": []byte(fmt.Sprintf(`{
			"protocols":["5.0"],
			"os":"darwin",
			"arch":"arm64",
			"filename":%q,
			"download_url":"https://github.com/provider.zip",
			"shasums_url":"https://github.com/provider_SHA256SUMS",
			"shasums_signature_url":"https://github.com/provider_SHA256SUMS.sig",
			"shasum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"signing_keys":{"gpg_public_keys":[{"key_id":"test","ascii_armor":%q}]}
		}`, filename, armoredEntity(t, entity))),
		"/provider_SHA256SUMS":     sums,
		"/provider_SHA256SUMS.sig": signature,
		"/provider.zip":            archive,
	}

	resolver := registry.NewRegistryResolver(
		registry.WithHTTPClient(testRegistryClient(t, requests)),
		registry.WithRegistrySigningKeysForTests(),
		registry.WithPlatform("darwin", "arm64"),
	)
	source, _ := registry.ParseProviderSource("siderolabs/talos")
	version, _ := registry.ParseProviderVersion("0.11.0")

	err := expectResolveProviderError(resolver, source, version)
	if !strings.Contains(err.Error(), "registry shasum does not match signed manifest checksum") {
		t.Fatalf("error = %v", err)
	}
}

func expectResolveProviderError(resolver *registry.RegistryResolver, source registry.ProviderSource, version registry.ProviderVersion) error {
	_, err := resolver.ResolveProvider(context.Background(), source, version)
	if err == nil {
		return fmt.Errorf("expected error")
	}
	return err
}

func testRegistryClient(t *testing.T, responses map[string][]byte) *http.Client {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := server.Client().Transport
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		cloned := req.Clone(req.Context())
		cloned.URL.Scheme = serverURL.Scheme
		cloned.URL.Host = serverURL.Host
		cloned.Host = req.URL.Host
		return transport.RoundTrip(cloned)
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func armoredEntity(t *testing.T, entity *openpgp.Entity) string {
	t.Helper()

	var out bytes.Buffer
	writer, err := armor.Encode(&out, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := entity.Serialize(writer); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
