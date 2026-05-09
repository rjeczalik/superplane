package terraform

import (
	"time"

	log "github.com/sirupsen/logrus"
)

const terraformAuditComponent = "terraform-provider-runtime"

type AuditLogger struct {
	logger *log.Entry
}

func NewAuditLogger(logger *log.Entry) *AuditLogger {
	if logger == nil {
		logger = log.WithField("component", terraformAuditComponent)
	}
	return &AuditLogger{logger: logger.WithField("component", terraformAuditComponent)}
}

func (a *AuditLogger) LogBinaryDownload(providerName, providerSource, providerVersion, url string, bytes int64) {
	a.log("binary_download", log.Fields{
		"provider": providerName,
		"source":   providerSource,
		"version":  providerVersion,
		"url":      url,
		"bytes":    bytes,
	})
}

func (a *AuditLogger) LogProviderLaunch(providerName, providerSource, providerVersion string, protocolMajor int) {
	a.log("provider_launch", log.Fields{
		"provider":       providerName,
		"source":         providerSource,
		"version":        providerVersion,
		"protocol_major": protocolMajor,
	})
}

func (a *AuditLogger) LogConfigureRPC(providerName, providerSource, providerVersion, capability string) {
	a.log("configure_rpc", providerFields(providerName, providerSource, providerVersion, capability))
}

func (a *AuditLogger) LogCapabilityExecution(providerName, providerSource, providerVersion, capability, operation, status string, duration time.Duration) {
	fields := providerFields(providerName, providerSource, providerVersion, capability)
	fields["operation"] = operation
	fields["status"] = status
	fields["duration_ms"] = duration.Milliseconds()
	a.log("capability_execution", fields)
}

func (a *AuditLogger) LogStateRead(providerName, providerSource, providerVersion, capability, status string) {
	fields := providerFields(providerName, providerSource, providerVersion, capability)
	fields["status"] = status
	a.log("state_read", fields)
}

func (a *AuditLogger) LogStateWrite(providerName, providerSource, providerVersion, capability, status string) {
	fields := providerFields(providerName, providerSource, providerVersion, capability)
	fields["status"] = status
	a.log("state_write", fields)
}

func (a *AuditLogger) LogTOFUAcceptance(providerName, providerSource, providerVersion, fingerprint string) {
	a.log("tofu_acceptance", log.Fields{
		"provider":    providerName,
		"source":      providerSource,
		"version":     providerVersion,
		"fingerprint": fingerprint,
	})
}

func (a *AuditLogger) LogStateMigrationDecision(providerName, providerSource, providerVersion, capability, decision string, dryRun bool) {
	fields := providerFields(providerName, providerSource, providerVersion, capability)
	fields["decision"] = decision
	fields["dry_run"] = dryRun
	a.log("state_migration_decision", fields)
}

func (a *AuditLogger) log(event string, fields log.Fields) {
	if a == nil || a.logger == nil {
		return
	}
	fields["event"] = event
	a.logger.WithFields(fields).Info("terraform provider audit event")
}

func providerFields(providerName, providerSource, providerVersion, capability string) log.Fields {
	return log.Fields{
		"provider":   providerName,
		"source":     providerSource,
		"version":    providerVersion,
		"capability": capability,
	}
}
