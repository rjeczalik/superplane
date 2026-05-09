package terraform

import (
	"fmt"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLoggerEmitsStructuredLifecycleEvents(t *testing.T) {
	logger, hook := test.NewNullLogger()
	audit := NewAuditLogger(log.NewEntry(logger))

	audit.LogBinaryDownload("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "https://releases.hashicorp.com/provider.zip", 42)
	audit.LogProviderLaunch("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", 6)
	audit.LogConfigureRPC("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "cloudflare.zone.read")
	audit.LogCapabilityExecution("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "cloudflare.zone.read", "read", "success", 12*time.Millisecond)
	audit.LogStateRead("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "cloudflare.zone.read", "not_found")
	audit.LogStateWrite("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "cloudflare.zone.apply", "success")
	audit.LogTOFUAcceptance("example", "registry.terraform.io/example/example", "1.0.0", "ABC123")
	audit.LogStateMigrationDecision("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "cloudflare.zone.apply", "migrate", true)

	require.Len(t, hook.AllEntries(), 8)
	assert.Equal(t, "binary_download", hook.AllEntries()[0].Data["event"])
	assert.Equal(t, "provider_launch", hook.AllEntries()[1].Data["event"])
	assert.Equal(t, "configure_rpc", hook.AllEntries()[2].Data["event"])
	assert.Equal(t, "capability_execution", hook.AllEntries()[3].Data["event"])
	assert.Equal(t, "state_read", hook.AllEntries()[4].Data["event"])
	assert.Equal(t, "state_write", hook.AllEntries()[5].Data["event"])
	assert.Equal(t, "tofu_acceptance", hook.AllEntries()[6].Data["event"])
	assert.Equal(t, "state_migration_decision", hook.AllEntries()[7].Data["event"])
}

func TestAuditLoggerDoesNotLogSensitivePayloads(t *testing.T) {
	logger, hook := test.NewNullLogger()
	audit := NewAuditLogger(log.NewEntry(logger))

	audit.LogConfigureRPC("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "cloudflare.zone.apply")
	audit.LogCapabilityExecution("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "cloudflare.zone.apply", "apply", "failed", time.Millisecond)
	audit.LogStateWrite("cloudflare", "registry.terraform.io/cloudflare/cloudflare", "4.52.0", "cloudflare.zone.apply", "failed")

	for _, entry := range hook.AllEntries() {
		serialized := entry.Message
		for key, value := range entry.Data {
			serialized += " " + key + "=" + strings.TrimSpace(toString(value))
		}
		assert.NotContains(t, serialized, "secret-token")
		assert.NotContains(t, serialized, "CF_API_TOKEN")
		assert.NotContains(t, serialized, "terraform.tfstate")
		assert.NotContains(t, serialized, "provider stderr")
		assert.NotContains(t, serialized, `{"password"`)
	}
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(fmt.Sprint(value), "\n", " "))
}
