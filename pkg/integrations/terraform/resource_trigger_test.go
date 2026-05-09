package terraform

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/core"
)

func TestGeneratedResourceTriggerContextInputsParsesChangedFields(t *testing.T) {
	integrationID := uuid.New()
	trigger := NewGeneratedResourceTrigger("example", "server", "example_server")

	_, _, _, cfg, err := trigger.contextInputs(core.TriggerContext{
		OrganizationID: uuid.NewString(),
		WorkflowID:     uuid.NewString(),
		NodeID:         "node-1",
		Integration:    &resourceTriggerIntegrationContext{id: integrationID},
		Configuration: map[string]any{
			"poll_interval_secs": float64(120),
			"changed_fields":     []any{"name", "status"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 120, cfg.PollIntervalSecs)
	assert.Equal(t, []string{"name", "status"}, cfg.ChangedFields)
}

type resourceTriggerIntegrationContext struct {
	mockIntegrationContextWithCapabilities
	id uuid.UUID
}

func (c *resourceTriggerIntegrationContext) ID() uuid.UUID {
	return c.id
}
