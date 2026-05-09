package registry

import (
	"fmt"
	"strings"
)

type ProviderSource struct {
	host      string
	namespace string
	typ       string
}

func ParseProviderSource(raw string) (ProviderSource, error) {
	if raw == "" {
		return ProviderSource{}, fmt.Errorf("provider source is empty")
	}

	parts := strings.Split(raw, "/")
	if len(parts) == 2 {
		parts = append([]string{"registry.terraform.io"}, parts...)
	}
	if len(parts) != 3 {
		return ProviderSource{}, fmt.Errorf("provider source %q must match <host>/<namespace>/<type> or <namespace>/<type>", raw)
	}
	for _, part := range parts {
		if part == "" {
			return ProviderSource{}, fmt.Errorf("provider source %q must not contain empty host, namespace, or type", raw)
		}
	}
	if !IsAllowedRegistryHost(parts[0]) {
		return ProviderSource{}, fmt.Errorf("provider source host %q is not in the registry allowlist", parts[0])
	}

	return ProviderSource{host: parts[0], namespace: parts[1], typ: parts[2]}, nil
}

func (s ProviderSource) Host() string {
	return s.host
}

func (s ProviderSource) Namespace() string {
	return s.namespace
}

func (s ProviderSource) Type() string {
	return s.typ
}

func (s ProviderSource) String() string {
	return s.host + "/" + s.namespace + "/" + s.typ
}
