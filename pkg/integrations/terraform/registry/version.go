package registry

import (
	"fmt"
	"regexp"
)

var providerVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type ProviderVersion struct {
	value string
}

func ParseProviderVersion(raw string) (ProviderVersion, error) {
	if raw == "" {
		return ProviderVersion{}, fmt.Errorf("provider version is empty")
	}
	if !providerVersionPattern.MatchString(raw) {
		return ProviderVersion{}, fmt.Errorf("provider version %q must be an exact semantic version", raw)
	}

	return ProviderVersion{value: raw}, nil
}

func (v ProviderVersion) String() string {
	return v.value
}
