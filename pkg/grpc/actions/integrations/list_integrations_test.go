package integrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/core"
	pb "github.com/superplanehq/superplane/pkg/protos/integrations"
	"github.com/superplanehq/superplane/pkg/registry"
	"github.com/superplanehq/superplane/test/support/impl"
)

type originInfoIntegration struct {
	core.Integration
	origin  string
	source  string
	version string
}

func (i *originInfoIntegration) Origin() string  { return i.origin }
func (i *originInfoIntegration) Source() string  { return i.source }
func (i *originInfoIntegration) Version() string { return i.version }

func TestSerializeIntegrations_Origin(t *testing.T) {
	t.Run("native integration defaults to empty origin/source/version", func(t *testing.T) {
		native := impl.NewDummyIntegration(impl.DummyIntegrationOptions{})
		r := &registry.Registry{
			Integrations: map[string]core.Integration{
				"nativeapp": registry.NewPanicableIntegration(native),
			},
		}

		defs := serializeIntegrations(r, []core.Integration{r.Integrations["nativeapp"]})
		require.Len(t, defs, 1)
		assert.Equal(t, "native", defs[0].Origin)
		assert.Equal(t, "", defs[0].Source)
		assert.Equal(t, "", defs[0].Version)
	})

	t.Run("terraform integration exposes origin metadata", func(t *testing.T) {
		tf := &originInfoIntegration{
			Integration: impl.NewDummyIntegration(impl.DummyIntegrationOptions{}),
			origin:      "terraform",
			source:      "registry.terraform.io/siderolabs/talos",
			version:     "0.11.0",
		}
		r := &registry.Registry{
			Integrations: map[string]core.Integration{
				"talos": registry.NewPanicableIntegration(tf),
			},
		}

		defs := serializeIntegrations(r, []core.Integration{r.Integrations["talos"]})
		require.Len(t, defs, 1)
		assert.Equal(t, "terraform", defs[0].Origin)
		assert.Equal(t, "registry.terraform.io/siderolabs/talos", defs[0].Source)
		assert.Equal(t, "0.11.0", defs[0].Version)
	})

	t.Run("wrapped terraform integration still satisfies OriginInfo", func(t *testing.T) {
		tf := &originInfoIntegration{
			Integration: impl.NewDummyIntegration(impl.DummyIntegrationOptions{}),
			origin:      "terraform",
			source:      "registry.terraform.io/siderolabs/talos",
			version:     "0.11.0",
		}
		wrapped := registry.NewPanicableIntegration(tf)

		origin, ok := wrapped.(core.OriginInfo)
		require.True(t, ok, "wrapped integration should satisfy OriginInfo")
		assert.Equal(t, "terraform", origin.Origin())
		assert.Equal(t, "registry.terraform.io/siderolabs/talos", origin.Source())
		assert.Equal(t, "0.11.0", origin.Version())
	})
}

func TestSerializeIntegrations_UsesSetupProviderTriggerCapabilities(t *testing.T) {
	integration := impl.NewDummyIntegration(impl.DummyIntegrationOptions{})
	setupProvider := &listIntegrationsSetupProvider{
		groups: []core.CapabilityGroup{
			{
				Label: "Resource Triggers",
				Capabilities: []core.Capability{
					{
						Type:  core.IntegrationCapabilityTypeTrigger,
						Name:  "terraform-google.storageBucket.onChanged",
						Label: "On Storage Bucket Changed",
					},
				},
			},
		},
	}
	r := &registry.Registry{
		Integrations: map[string]core.Integration{
			"dummy": registry.NewPanicableIntegration(integration),
		},
		SetupProviders: map[string]core.IntegrationSetupProvider{
			"dummy": setupProvider,
		},
	}

	defs := serializeIntegrations(r, []core.Integration{r.Integrations["dummy"]})
	require.Len(t, defs, 1)
	require.Len(t, defs[0].Capabilities, 1)
	assert.Equal(t, pb.CapabilityDefinition_TYPE_TRIGGER, defs[0].Capabilities[0].Type)
	assert.Equal(t, "terraform-google.storageBucket.onChanged", defs[0].Capabilities[0].Name)
	assert.Equal(t, "Resource Triggers", defs[0].CapabilityGroups[0].Label)
	assert.Equal(t, []string{"terraform-google.storageBucket.onChanged"}, defs[0].CapabilityGroups[0].Capabilities)
}

type listIntegrationsSetupProvider struct {
	groups []core.CapabilityGroup
}

func (p *listIntegrationsSetupProvider) CapabilityGroups() []core.CapabilityGroup {
	return p.groups
}

func (p *listIntegrationsSetupProvider) FirstStep(ctx core.SetupStepContext) core.SetupStep {
	return core.SetupStep{}
}

func (p *listIntegrationsSetupProvider) OnStepSubmit(ctx core.SetupStepContext) (*core.SetupStep, error) {
	return nil, nil
}

func (p *listIntegrationsSetupProvider) OnStepRevert(ctx core.SetupStepContext) error {
	return nil
}

func (p *listIntegrationsSetupProvider) OnPropertyUpdate(ctx core.PropertyUpdateContext) (*core.SetupStep, error) {
	return nil, nil
}

func (p *listIntegrationsSetupProvider) OnSecretUpdate(ctx core.SecretUpdateContext) (*core.SetupStep, error) {
	return nil, nil
}

func (p *listIntegrationsSetupProvider) OnCapabilityUpdate(ctx core.CapabilityUpdateContext) (*core.SetupStep, error) {
	return nil, nil
}
