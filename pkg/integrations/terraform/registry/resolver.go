package registry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"

	"github.com/ProtonMail/go-crypto/openpgp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const telemetryName = "github.com/superplanehq/superplane/pkg/integrations/terraform"
const maxRegistryResponseBytes = 512 << 20

var registryTracer = otel.Tracer(telemetryName)

type RegistryResolver struct {
	httpClient                *http.Client
	trustedKeys               openpgp.EntityList
	keyPinStore               *KeyPinStore
	keyPinPolicy              KeyPinPolicy
	allowRegistrySigningKeys  bool
	skipSignatureVerification bool
	os                        string
	arch                      string
}

type ResolverOption func(*RegistryResolver)

type ProviderPackage struct {
	Source              ProviderSource
	Version             ProviderVersion
	OS                  string
	Arch                string
	Platform            string
	Filename            string
	DownloadURL         string
	SHASumsURL          string
	SHASumsSignatureURL string
	Protocols           []string
	ProtocolMajor       int
	SHA256              string
	Archive             []byte
}

func NewRegistryResolver(opts ...ResolverOption) *RegistryResolver {
	resolver := &RegistryResolver{
		httpClient: withRedirectPolicy(http.DefaultClient),
		os:         runtime.GOOS,
		arch:       runtime.GOARCH,
	}
	for _, opt := range opts {
		opt(resolver)
	}
	return resolver
}

func WithHTTPClient(client *http.Client) ResolverOption {
	return func(r *RegistryResolver) {
		if client != nil {
			r.httpClient = withRedirectPolicy(client)
		}
	}
}

func WithTrustedKeys(keys openpgp.EntityList) ResolverOption {
	return func(r *RegistryResolver) {
		r.trustedKeys = keys
	}
}

func WithKeyPinStore(store *KeyPinStore, policy KeyPinPolicy) ResolverOption {
	return func(r *RegistryResolver) {
		r.keyPinStore = store
		r.keyPinPolicy = policy
	}
}

func WithSignatureVerificationSkipped() ResolverOption {
	return func(r *RegistryResolver) {
		r.skipSignatureVerification = true
	}
}

func WithRegistrySigningKeysForTests() ResolverOption {
	return func(r *RegistryResolver) {
		r.allowRegistrySigningKeys = true
	}
}

func WithPlatform(osName, arch string) ResolverOption {
	return func(r *RegistryResolver) {
		if osName != "" {
			r.os = osName
		}
		if arch != "" {
			r.arch = arch
		}
	}
}

func (r *RegistryResolver) ResolveProvider(ctx context.Context, source ProviderSource, version ProviderVersion) (*ProviderPackage, error) {
	ctx, span := registryTracer.Start(ctx, "terraform.provider.registry_resolve", trace.WithAttributes(
		attribute.String("source", source.String()),
		attribute.String("version", version.String()),
	))
	var spanErr error
	defer func() {
		if spanErr != nil {
			span.RecordError(spanErr)
			span.SetStatus(codes.Error, "error")
		}
		span.End()
	}()

	responseURL := fmt.Sprintf(
		"https://%s/v1/providers/%s/%s/%s/download/%s/%s",
		source.Host(),
		source.Namespace(),
		source.Type(),
		version.String(),
		r.os,
		r.arch,
	)

	responseBytes, err := r.fetch(ctx, responseURL)
	if err != nil {
		spanErr = err
		return nil, fmt.Errorf("fetch provider package metadata: %w", err)
	}
	response, err := ParsePackageResponse(responseBytes, responseURL)
	if err != nil {
		spanErr = err
		return nil, err
	}

	sums, err := r.fetch(ctx, response.SHASumsURL)
	if err != nil {
		spanErr = err
		return nil, fmt.Errorf("fetch SHA256SUMS: %w", err)
	}
	if !r.skipSignatureVerification {
		signature, err := r.fetch(ctx, response.SHASumsSignatureURL)
		if err != nil {
			spanErr = err
			return nil, fmt.Errorf("fetch SHA256SUMS signature: %w", err)
		}
		keys, err := r.signingKeys(source, response)
		if err != nil {
			spanErr = err
			return nil, err
		}
		if err := VerifySignature(sums, signature, keys); err != nil {
			spanErr = err
			return nil, err
		}
	}

	checksum, err := VerifyPackageChecksumBinding(sums, response.Filename, response.SHASum)
	if err != nil {
		spanErr = err
		return nil, err
	}

	archive, err := r.fetch(ctx, response.DownloadURL)
	if err != nil {
		spanErr = err
		return nil, fmt.Errorf("fetch provider archive: %w", err)
	}
	if err := VerifyChecksum(bytes.NewReader(archive), checksum); err != nil {
		spanErr = err
		return nil, err
	}

	return &ProviderPackage{
		Source:              source,
		Version:             version,
		OS:                  response.OS,
		Arch:                response.Arch,
		Platform:            response.OS + "_" + response.Arch,
		Filename:            response.Filename,
		DownloadURL:         response.DownloadURL,
		SHASumsURL:          response.SHASumsURL,
		SHASumsSignatureURL: response.SHASumsSignatureURL,
		Protocols:           response.Protocols,
		ProtocolMajor:       response.ProtocolMajor,
		SHA256:              checksum,
		Archive:             archive,
	}, nil
}

func (r *RegistryResolver) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, span := registryTracer.Start(ctx, "terraform.provider.registry_fetch", trace.WithAttributes(attribute.String("url_host", hostForTelemetry(rawURL))))
	var spanErr error
	defer func() {
		if spanErr != nil {
			span.RecordError(spanErr)
			span.SetStatus(codes.Error, "error")
		}
		span.End()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		spanErr = err
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		spanErr = err
		return nil, err
	}
	defer resp.Body.Close()

	if resp.ContentLength > maxRegistryResponseBytes {
		spanErr = fmt.Errorf("registry response exceeds %d bytes", maxRegistryResponseBytes)
		return nil, spanErr
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		spanErr = fmt.Errorf("GET %s returned %s", rawURL, resp.Status)
		return nil, spanErr
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistryResponseBytes+1))
	if err != nil {
		spanErr = err
		return nil, err
	}
	if len(body) > maxRegistryResponseBytes {
		spanErr = fmt.Errorf("registry response exceeds %d bytes", maxRegistryResponseBytes)
		return nil, spanErr
	}
	return body, nil
}

func (r *RegistryResolver) signingKeys(source ProviderSource, response *PackageResponse) (openpgp.EntityList, error) {
	if len(r.trustedKeys) > 0 {
		return r.trustedKeys, nil
	}
	var keys openpgp.EntityList
	for _, signingKey := range response.SigningKeys {
		parsed, err := ParseArmoredKey([]byte(signingKey.ASCIIArmor))
		if err != nil {
			return nil, err
		}
		keys = append(keys, parsed...)
	}
	if r.keyPinStore != nil {
		return r.keyPinStore.ResolveKey(source, keys, r.keyPinPolicy)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no trusted GPG keys configured for provider signature verification")
	}
	if !r.allowRegistrySigningKeys {
		return nil, fmt.Errorf("registry-provided signing keys are not trusted without an explicit key pin or test override")
	}
	return keys, nil
}

func withRedirectPolicy(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	previous := client.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("redirect target must use https")
		}
		host := req.URL.Hostname()
		if !IsAllowedRegistryHost(host) && !IsAllowedDownloadHost(host) {
			return fmt.Errorf("redirect target host %q is not allowlisted", host)
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &clone
}

func hostForTelemetry(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}
