package terraform

import (
	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

func runnerTestAction() *GeneratedAction {
	return &GeneratedAction{
		integrationName: "talos",
		resourceName:    "machineSecrets",
		op:              "read",
		inputSchema:     []configuration.Field{{Name: "machine_type", Type: configuration.FieldTypeString}},
		outputSchema: []configuration.Field{
			{Name: "id", Type: configuration.FieldTypeString},
			{Name: "password", Type: configuration.FieldTypeString, Sensitive: true},
		},
		sensitiveAttrs:  map[string]struct{}{"password": {}},
		capabilityName:  "talos.machineSecrets.read",
		schemaHash:      "hash",
		providerName:    "talos",
		providerSource:  "registry.terraform.io/siderolabs/talos",
		providerVersion: "0.11.0",
	}
}

type recordingExecutionState struct {
	channel     string
	payloadType string
	payloads    []any
}

func (s *recordingExecutionState) IsFinished() bool              { return false }
func (s *recordingExecutionState) SetKV(key, value string) error { return nil }
func (s *recordingExecutionState) Emit(channel, payloadType string, payloads []any) error {
	s.channel = channel
	s.payloadType = payloadType
	s.payloads = payloads
	return nil
}
func (s *recordingExecutionState) Pass() error                       { return nil }
func (s *recordingExecutionState) Fail(reason, message string) error { return nil }

type runnerIntegrationContext struct {
	mockIntegrationContextWithCapabilities
	secrets     core.IntegrationSecretStorage
	setSecrets  map[string][]byte
	listSecrets []core.IntegrationSecret
}

func (c *runnerIntegrationContext) Secrets() core.IntegrationSecretStorage { return c.secrets }
func (c *runnerIntegrationContext) ID() uuid.UUID                          { return uuid.New() }
func (c *runnerIntegrationContext) SetSecret(name string, value []byte) error {
	if c.setSecrets == nil {
		c.setSecrets = map[string][]byte{}
	}
	c.setSecrets[name] = value
	return nil
}
func (c *runnerIntegrationContext) GetSecrets() ([]core.IntegrationSecret, error) {
	return c.listSecrets, nil
}
