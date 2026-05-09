package registry

var registryHostAllowlist = map[string]struct{}{
	"registry.terraform.io": {},
}

var downloadHostAllowlist = map[string]struct{}{
	"releases.hashicorp.com":               {},
	"github.com":                           {},
	"release-assets.githubusercontent.com": {},
}

func IsAllowedRegistryHost(host string) bool {
	_, ok := registryHostAllowlist[host]
	return ok
}

func IsAllowedDownloadHost(host string) bool {
	_, ok := downloadHostAllowlist[host]
	return ok
}
