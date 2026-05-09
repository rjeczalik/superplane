package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type PackageResponse struct {
	Protocols           []string
	OS                  string
	Arch                string
	Filename            string
	DownloadURL         string
	SHASumsURL          string
	SHASumsSignatureURL string
	SHASum              string
	ProtocolMajor       int
	SigningKeys         []SigningKey
}

type SigningKey struct {
	KeyID      string
	ASCIIArmor string
}

type packageResponseJSON struct {
	Protocols           []string `json:"protocols"`
	OS                  string   `json:"os"`
	Arch                string   `json:"arch"`
	Filename            string   `json:"filename"`
	DownloadURL         string   `json:"download_url"`
	SHASumsURL          string   `json:"shasums_url"`
	SHASumsSignatureURL string   `json:"shasums_signature_url"`
	SHASum              string   `json:"shasum"`
	SigningKeys         struct {
		GPGPublicKeys []signingKeyJSON `json:"gpg_public_keys"`
	} `json:"signing_keys"`
}

type signingKeyJSON struct {
	KeyID          string `json:"key_id"`
	ASCIIArmor     string `json:"ascii_armor"`
	Source         string `json:"source"`
	SourceURL      string `json:"source_url"`
	TrustSignature string `json:"trust_signature"`
}

func ParsePackageResponse(raw []byte, responseURL string) (*PackageResponse, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var parsed packageResponseJSON
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode registry package response: %w", err)
	}

	protocolMajor, err := selectedProtocolMajor(parsed.Protocols)
	if err != nil {
		return nil, err
	}

	downloadURL, err := resolveDownloadURL(responseURL, parsed.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("download_url: %w", err)
	}
	shasumsURL, err := resolveDownloadURL(responseURL, parsed.SHASumsURL)
	if err != nil {
		return nil, fmt.Errorf("shasums_url: %w", err)
	}
	signatureURL, err := resolveDownloadURL(responseURL, parsed.SHASumsSignatureURL)
	if err != nil {
		return nil, fmt.Errorf("shasums_signature_url: %w", err)
	}

	return &PackageResponse{
		Protocols:           parsed.Protocols,
		OS:                  parsed.OS,
		Arch:                parsed.Arch,
		Filename:            parsed.Filename,
		DownloadURL:         downloadURL,
		SHASumsURL:          shasumsURL,
		SHASumsSignatureURL: signatureURL,
		SHASum:              parsed.SHASum,
		ProtocolMajor:       protocolMajor,
		SigningKeys:         signingKeys(parsed.SigningKeys.GPGPublicKeys),
	}, nil
}

func signingKeys(raw []signingKeyJSON) []SigningKey {
	result := make([]SigningKey, 0, len(raw))
	for _, key := range raw {
		result = append(result, SigningKey{KeyID: key.KeyID, ASCIIArmor: key.ASCIIArmor})
	}
	return result
}

func selectedProtocolMajor(protocols []string) (int, error) {
	best := 0
	for _, protocol := range protocols {
		major, _, ok := strings.Cut(protocol, ".")
		if !ok {
			continue
		}
		switch major {
		case "6":
			return 6, nil
		case "5":
			best = 5
		}
	}
	if best == 0 {
		return 0, fmt.Errorf("registry response does not advertise Terraform protocol v5 or v6")
	}

	return best, nil
}

func resolveDownloadURL(baseRaw string, raw string) (string, error) {
	base, err := url.Parse(baseRaw)
	if err != nil {
		return "", fmt.Errorf("parse response URL: %w", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	resolved := base.ResolveReference(parsed)
	if resolved.Scheme != "https" {
		return "", fmt.Errorf("URL must use https")
	}
	if !IsAllowedDownloadHost(resolved.Hostname()) {
		return "", fmt.Errorf("URL host %q is not in the download allowlist", resolved.Hostname())
	}
	if base.Scheme == "http" && resolved.Scheme == "https" {
		return "", fmt.Errorf("HTTP-to-HTTPS response URL upgrade is not allowed")
	}

	return resolved.String(), nil
}
