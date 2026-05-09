package hetznerrobot

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/test/support/contexts"
	"github.com/superplanehq/superplane/test/support/logger"
)

// expectedCapabilities is the canonical, ordered list of all 16 capability
// names exposed by the Hetzner Robot integration.
var expectedCapabilities = []string{
	"hetznerRobot.listServers",
	"hetznerRobot.getServer",
	"hetznerRobot.renameServer",
	"hetznerRobot.resetServer",
	"hetznerRobot.wakeOnLan",
	"hetznerRobot.listSshKeys",
	"hetznerRobot.addSshKey",
	"hetznerRobot.deleteSshKey",
	"hetznerRobot.enableRescue",
	"hetznerRobot.disableRescue",
	"hetznerRobot.installLinux",
	"hetznerRobot.cancelLinuxInstall",
	"hetznerRobot.listFirewallRules",
	"hetznerRobot.addFirewallRule",
	"hetznerRobot.updateFirewallRule",
	"hetznerRobot.deleteFirewallRule",
}

func okResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func errorResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func Test_SetupProvider_CapabilityGroups(t *testing.T) {
	s := &SetupProvider{}
	groups := s.CapabilityGroups()

	require.Len(t, groups, 5, "expected exactly 5 capability groups")
	assert.Equal(t, "Server", groups[0].Label)
	assert.Equal(t, "SSH Keys", groups[1].Label)
	assert.Equal(t, "Rescue", groups[2].Label)
	assert.Equal(t, "Linux Installation", groups[3].Label)
	assert.Equal(t, "Firewall", groups[4].Label)

	assert.ElementsMatch(t, expectedCapabilities, allCapabilityNames(groups))

	// Every capability has a non-empty label and description, and is an action.
	for _, group := range groups {
		for _, cap := range group.Capabilities {
			assert.Equal(t, core.IntegrationCapabilityTypeAction, cap.Type, cap.Name)
			assert.NotEmpty(t, cap.Label, cap.Name)
			assert.NotEmpty(t, cap.Description, cap.Name)
		}
	}
}

func Test_SetupProvider_FirstStep(t *testing.T) {
	s := &SetupProvider{}
	step := s.FirstStep(core.SetupStepContext{})
	assert.Equal(t, core.SetupStepTypeCapabilitySelection, step.Type)
	assert.Equal(t, SetupStepCapabilitySelection, step.Name)
	assert.ElementsMatch(t, expectedCapabilities, step.Capabilities)
}

func Test_SetupProvider_OnStepSubmit_CapabilitySelection(t *testing.T) {
	s := &SetupProvider{}
	log := logger.DiscardLogger()

	t.Run("empty selection returns error", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{}
		_, err := s.OnStepSubmit(core.SetupStepContext{
			Step:         core.StepInfo{Name: SetupStepCapabilitySelection},
			Logger:       log,
			Capabilities: capCtx,
		})
		require.Error(t, err)
		assert.Empty(t, capCtx.RequestedCapabilties)
	})

	t.Run("unknown capability returns error and does not request", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{}
		_, err := s.OnStepSubmit(core.SetupStepContext{
			Step:         core.StepInfo{Name: SetupStepCapabilitySelection, Capabilities: []string{"hetznerRobot.bogus"}},
			Logger:       log,
			Capabilities: capCtx,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown capability")
		assert.Empty(t, capCtx.RequestedCapabilties)
	})

	t.Run("read-only selection returns credentials step with read-only instructions", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{}
		next, err := s.OnStepSubmit(core.SetupStepContext{
			Step: core.StepInfo{
				Name:         SetupStepCapabilitySelection,
				Capabilities: []string{"hetznerRobot.listServers", "hetznerRobot.getServer"},
			},
			Logger:       log,
			Capabilities: capCtx,
		})
		require.NoError(t, err)
		require.NotNil(t, next)
		assert.Equal(t, SetupStepEnterCredentials, next.Name)
		assert.Equal(t, core.SetupStepTypeInputs, next.Type)
		assert.Contains(t, next.Instructions, "Read-only")
		assert.NotContains(t, next.Instructions, "modify")
		assert.ElementsMatch(t, []string{"hetznerRobot.listServers", "hetznerRobot.getServer"}, capCtx.RequestedCapabilties)
		// Remaining 14 should be marked Available.
		assert.Len(t, capCtx.AvailableCapabilities, len(expectedCapabilities)-2)
	})

	t.Run("mutating selection returns credentials step with read-write instructions", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{}
		next, err := s.OnStepSubmit(core.SetupStepContext{
			Step: core.StepInfo{
				Name:         SetupStepCapabilitySelection,
				Capabilities: []string{"hetznerRobot.listServers", "hetznerRobot.renameServer"},
			},
			Logger:       log,
			Capabilities: capCtx,
		})
		require.NoError(t, err)
		require.NotNil(t, next)
		assert.Contains(t, next.Instructions, "Read & Write")
		assert.Contains(t, next.Instructions, "modify")
	})
}

func Test_SetupProvider_OnStepSubmit_EnterCredentials(t *testing.T) {
	s := &SetupProvider{}
	log := logger.DiscardLogger()

	makeStep := func(inputs any, capCtx *contexts.CapabilityContext) (core.SetupStepContext, *contexts.IntegrationContext, *contexts.IntegrationPropertyStorage, *contexts.HTTPContext) {
		intCtx := &contexts.IntegrationContext{NewSetupFlow: true}
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		httpCtx := &contexts.HTTPContext{}
		return core.SetupStepContext{
			Step:         core.StepInfo{Name: SetupStepEnterCredentials, Inputs: inputs},
			Logger:       log,
			HTTP:         httpCtx,
			Secrets:      intCtx.Secrets(),
			Properties:   props,
			Capabilities: capCtx,
		}, intCtx, props, httpCtx
	}

	t.Run("missing username returns error", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{RequestedCapabilties: []string{"hetznerRobot.listServers"}}
		stepCtx, intCtx, props, _ := makeStep(map[string]any{"password": "p"}, capCtx)
		_, err := s.OnStepSubmit(stepCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "username is required")
		assert.Empty(t, intCtx.CurrentSecrets)
		_, perr := props.GetString(PropertyCredentialPermission)
		assert.Error(t, perr)
	})

	t.Run("non-string password returns error", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{RequestedCapabilties: []string{"hetznerRobot.listServers"}}
		stepCtx, _, _, _ := makeStep(map[string]any{"username": "u", "password": 123}, capCtx)
		_, err := s.OnStepSubmit(stepCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "password must be a string")
	})

	t.Run("empty username returns error", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{RequestedCapabilties: []string{"hetznerRobot.listServers"}}
		stepCtx, _, _, _ := makeStep(map[string]any{"username": "   ", "password": "p"}, capCtx)
		_, err := s.OnStepSubmit(stepCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "username is required")
	})

	t.Run("bad credentials do not create secrets or properties", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{RequestedCapabilties: []string{"hetznerRobot.listServers"}}
		stepCtx, intCtx, props, httpCtx := makeStep(map[string]any{"username": "u", "password": "p"}, capCtx)
		httpCtx.Responses = []*http.Response{
			errorResponse(http.StatusUnauthorized, `{"error":{"code":"UNAUTHORIZED","message":"bad creds"}}`),
		}
		_, err := s.OnStepSubmit(stepCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verify credentials")
		assert.Empty(t, intCtx.CurrentSecrets)
		_, perr := props.GetString(PropertyCredentialPermission)
		assert.Error(t, perr)
		assert.Empty(t, capCtx.EnabledCapabilities)
	})

	t.Run("read-only selection creates credentialPermission=readOnly", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{RequestedCapabilties: []string{"hetznerRobot.listServers"}}
		stepCtx, intCtx, props, httpCtx := makeStep(map[string]any{"username": "u", "password": "p"}, capCtx)
		httpCtx.Responses = []*http.Response{
			okResponse(`[]`), // Verify
			okResponse(`[]`), // ListServers
		}
		next, err := s.OnStepSubmit(stepCtx)
		require.NoError(t, err)
		require.NotNil(t, next)
		assert.Equal(t, core.SetupStepTypeDone, next.Type)
		assert.Equal(t, SetupStepDone, next.Name)
		perm, err := props.GetString(PropertyCredentialPermission)
		require.NoError(t, err)
		assert.Equal(t, CredentialPermissionReadOnly, perm)
		// Secrets stored.
		uVal, err := intCtx.Secrets().Get(SecretUsername)
		require.NoError(t, err)
		assert.Equal(t, "u", uVal)
		pVal, err := intCtx.Secrets().Get(SecretPassword)
		require.NoError(t, err)
		assert.Equal(t, "p", pVal)
		// Capabilities enabled.
		assert.Equal(t, []string{"hetznerRobot.listServers"}, capCtx.EnabledCapabilities)
		assert.Contains(t, next.Instructions, "did not find any dedicated servers")
	})

	t.Run("mutating selection creates credentialPermission=readWrite and shows server count", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{RequestedCapabilties: []string{"hetznerRobot.renameServer"}}
		stepCtx, _, props, httpCtx := makeStep(map[string]any{"username": "u", "password": "p"}, capCtx)
		httpCtx.Responses = []*http.Response{
			okResponse(`[]`), // Verify
			okResponse(`[{"server":{"server_number":"1"}},{"server":{"server_number":"2"}},{"server":{"server_number":"3"}}]`),
		}
		next, err := s.OnStepSubmit(stepCtx)
		require.NoError(t, err)
		perm, err := props.GetString(PropertyCredentialPermission)
		require.NoError(t, err)
		assert.Equal(t, CredentialPermissionReadWrite, perm)
		assert.Contains(t, next.Instructions, "3 dedicated servers")
	})
}

func Test_SetupProvider_OnStepSubmit_UpgradeCredentials(t *testing.T) {
	s := &SetupProvider{}
	log := logger.DiscardLogger()

	intCtx := &contexts.IntegrationContext{
		NewSetupFlow: true,
		CurrentSecrets: map[string]core.IntegrationSecret{
			SecretUsername: {Name: SecretUsername, Value: []byte("old-u")},
			SecretPassword: {Name: SecretPassword, Value: []byte("old-p")},
		},
	}
	props := contexts.NewIntegrationPropertyStorage(intCtx)
	require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
		Name: PropertyCredentialPermission, Value: CredentialPermissionReadOnly,
	}))
	capCtx := &contexts.CapabilityContext{RequestedCapabilties: []string{"hetznerRobot.renameServer"}}
	httpCtx := &contexts.HTTPContext{
		Responses: []*http.Response{
			okResponse(`[]`), // Verify
			okResponse(`[{"server":{"server_number":"1"}},{"server":{"server_number":"2"}}]`), // ListServers
		},
	}

	next, err := s.OnStepSubmit(core.SetupStepContext{
		Step:         core.StepInfo{Name: SetupStepUpgradeCredentials, Inputs: map[string]any{"username": "new-u", "password": "new-p"}},
		Logger:       log,
		HTTP:         httpCtx,
		Secrets:      intCtx.Secrets(),
		Properties:   props,
		Capabilities: capCtx,
	})
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, core.SetupStepTypeDone, next.Type)

	// Secrets updated.
	u, err := intCtx.Secrets().Get(SecretUsername)
	require.NoError(t, err)
	assert.Equal(t, "new-u", u)
	p, err := intCtx.Secrets().Get(SecretPassword)
	require.NoError(t, err)
	assert.Equal(t, "new-p", p)

	// Permission upgraded.
	perm, err := props.GetString(PropertyCredentialPermission)
	require.NoError(t, err)
	assert.Equal(t, CredentialPermissionReadWrite, perm)

	// Requested capabilities enabled.
	assert.Equal(t, []string{"hetznerRobot.renameServer"}, capCtx.EnabledCapabilities)
}

func Test_SetupProvider_OnStepSubmit_UnknownStep(t *testing.T) {
	s := &SetupProvider{}
	_, err := s.OnStepSubmit(core.SetupStepContext{
		Step: core.StepInfo{Name: "bogus"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown step")
}

func Test_SetupProvider_OnSecretUpdate(t *testing.T) {
	s := &SetupProvider{}
	log := logger.DiscardLogger()

	t.Run("unknown secret returns error", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{NewSetupFlow: true}
		_, err := s.OnSecretUpdate(core.SecretUpdateContext{
			Logger:     log,
			SecretName: "other",
			Value:      "x",
			HTTP:       &contexts.HTTPContext{},
			Properties: contexts.NewIntegrationPropertyStorage(intCtx),
			Secrets:    intCtx.Secrets(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown secret")
	})

	t.Run("missing/empty value returns error", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{NewSetupFlow: true}
		_, err := s.OnSecretUpdate(core.SecretUpdateContext{
			Logger:     log,
			SecretName: SecretUsername,
			Value:      "   ",
			HTTP:       &contexts.HTTPContext{},
			Properties: contexts.NewIntegrationPropertyStorage(intCtx),
			Secrets:    intCtx.Secrets(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "username is required")
	})

	t.Run("read-only stored permission allows verified single-secret rotation", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				SecretUsername: {Name: SecretUsername, Value: []byte("old-u")},
				SecretPassword: {Name: SecretPassword, Value: []byte("p")},
			},
		}
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
			Name: PropertyCredentialPermission, Value: CredentialPermissionReadOnly,
		}))
		httpCtx := &contexts.HTTPContext{Responses: []*http.Response{okResponse(`[]`)}}

		next, err := s.OnSecretUpdate(core.SecretUpdateContext{
			Logger:     log,
			SecretName: SecretUsername,
			Value:      "new-u",
			HTTP:       httpCtx,
			Properties: props,
			Secrets:    intCtx.Secrets(),
		})
		require.NoError(t, err)
		assert.Nil(t, next)
		stored, err := intCtx.Secrets().Get(SecretUsername)
		require.NoError(t, err)
		assert.Equal(t, "new-u", stored)
	})

	t.Run("bad credentials do not update secrets", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				SecretUsername: {Name: SecretUsername, Value: []byte("old-u")},
				SecretPassword: {Name: SecretPassword, Value: []byte("p")},
			},
		}
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
			Name: PropertyCredentialPermission, Value: CredentialPermissionReadOnly,
		}))
		httpCtx := &contexts.HTTPContext{
			Responses: []*http.Response{errorResponse(http.StatusUnauthorized, `{"error":{"code":"UNAUTHORIZED","message":"bad"}}`)},
		}

		_, err := s.OnSecretUpdate(core.SecretUpdateContext{
			Logger:     log,
			SecretName: SecretUsername,
			Value:      "new-u",
			HTTP:       httpCtx,
			Properties: props,
			Secrets:    intCtx.Secrets(),
		})
		require.Error(t, err)
		stored, err := intCtx.Secrets().Get(SecretUsername)
		require.NoError(t, err)
		assert.Equal(t, "old-u", stored, "username must not be updated when verification fails")
	})

	t.Run("read-write stored permission returns upgradeCredentials and does not update secrets", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				SecretUsername: {Name: SecretUsername, Value: []byte("old-u")},
				SecretPassword: {Name: SecretPassword, Value: []byte("p")},
			},
		}
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
			Name: PropertyCredentialPermission, Value: CredentialPermissionReadWrite,
		}))
		httpCtx := &contexts.HTTPContext{}

		next, err := s.OnSecretUpdate(core.SecretUpdateContext{
			Logger:     log,
			SecretName: SecretUsername,
			Value:      "new-u",
			HTTP:       httpCtx,
			Properties: props,
			Secrets:    intCtx.Secrets(),
		})
		require.NoError(t, err)
		require.NotNil(t, next)
		assert.Equal(t, SetupStepUpgradeCredentials, next.Name)

		// Stored secret untouched.
		stored, err := intCtx.Secrets().Get(SecretUsername)
		require.NoError(t, err)
		assert.Equal(t, "old-u", stored)

		// No HTTP requests should have been made (no verification).
		assert.Empty(t, httpCtx.Requests)
	})
}

func Test_SetupProvider_OnCapabilityUpdate(t *testing.T) {
	s := &SetupProvider{}
	log := logger.DiscardLogger()

	t.Run("no requested capabilities returns error", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{}
		_, err := s.OnCapabilityUpdate(core.CapabilityUpdateContext{
			Logger:       log,
			Changes:      map[core.IntegrationCapabilityState][]string{},
			Capabilities: &contexts.CapabilityContext{},
			Properties:   contexts.NewIntegrationPropertyStorage(intCtx),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no requested capabilities")
	})

	t.Run("unknown requested capability returns error", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{}
		capCtx := &contexts.CapabilityContext{}
		_, err := s.OnCapabilityUpdate(core.CapabilityUpdateContext{
			Logger: log,
			Changes: map[core.IntegrationCapabilityState][]string{
				core.IntegrationCapabilityStateRequested: {"hetznerRobot.bogus"},
			},
			Capabilities: capCtx,
			Properties:   contexts.NewIntegrationPropertyStorage(intCtx),
		})
		require.Error(t, err)
		assert.Empty(t, capCtx.EnabledCapabilities)
	})

	t.Run("read-only stored credential enables read-only requested capability", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{}
		capCtx := &contexts.CapabilityContext{}
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
			Name: PropertyCredentialPermission, Value: CredentialPermissionReadOnly,
		}))

		next, err := s.OnCapabilityUpdate(core.CapabilityUpdateContext{
			Logger: log,
			Changes: map[core.IntegrationCapabilityState][]string{
				core.IntegrationCapabilityStateRequested: {"hetznerRobot.listServers"},
			},
			Capabilities: capCtx,
			Properties:   props,
		})
		require.NoError(t, err)
		assert.Nil(t, next)
		assert.Equal(t, []string{"hetznerRobot.listServers"}, capCtx.EnabledCapabilities)
	})

	t.Run("read-write stored credential enables mutating requested capability", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{}
		capCtx := &contexts.CapabilityContext{}
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
			Name: PropertyCredentialPermission, Value: CredentialPermissionReadWrite,
		}))

		next, err := s.OnCapabilityUpdate(core.CapabilityUpdateContext{
			Logger: log,
			Changes: map[core.IntegrationCapabilityState][]string{
				core.IntegrationCapabilityStateRequested: {"hetznerRobot.renameServer"},
			},
			Capabilities: capCtx,
			Properties:   props,
		})
		require.NoError(t, err)
		assert.Nil(t, next)
		assert.Equal(t, []string{"hetznerRobot.renameServer"}, capCtx.EnabledCapabilities)
	})

	t.Run("read-only stored plus mutating request returns upgradeCredentials", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{}
		capCtx := &contexts.CapabilityContext{}
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
			Name: PropertyCredentialPermission, Value: CredentialPermissionReadOnly,
		}))

		next, err := s.OnCapabilityUpdate(core.CapabilityUpdateContext{
			Logger: log,
			Changes: map[core.IntegrationCapabilityState][]string{
				core.IntegrationCapabilityStateRequested: {"hetznerRobot.renameServer"},
			},
			Capabilities: capCtx,
			Properties:   props,
		})
		require.NoError(t, err)
		require.NotNil(t, next)
		assert.Equal(t, SetupStepUpgradeCredentials, next.Name)
		assert.Empty(t, capCtx.EnabledCapabilities)
	})

	t.Run("mutating request remains requested through credential upgrade", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{
			NewSetupFlow: true,
			CurrentSecrets: map[string]core.IntegrationSecret{
				SecretUsername: {Name: SecretUsername, Value: []byte("old-u")},
				SecretPassword: {Name: SecretPassword, Value: []byte("old-p")},
			},
		}
		capCtx := &contexts.CapabilityContext{
			EnabledCapabilities: []string{"hetznerRobot.listServers"},
		}
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
			Name: PropertyCredentialPermission, Value: CredentialPermissionReadOnly,
		}))

		next, err := s.OnCapabilityUpdate(core.CapabilityUpdateContext{
			Logger: log,
			Changes: map[core.IntegrationCapabilityState][]string{
				core.IntegrationCapabilityStateRequested: {"hetznerRobot.renameServer"},
			},
			Capabilities: capCtx,
			Properties:   props,
		})
		require.NoError(t, err)
		require.NotNil(t, next)
		assert.Equal(t, SetupStepUpgradeCredentials, next.Name)
		assert.ElementsMatch(t, []string{"hetznerRobot.renameServer"}, capCtx.RequestedCapabilties)

		httpCtx := &contexts.HTTPContext{
			Responses: []*http.Response{
				okResponse(`[]`),
				okResponse(`[]`),
			},
		}
		done, err := s.OnStepSubmit(core.SetupStepContext{
			Step: core.StepInfo{
				Name:   SetupStepUpgradeCredentials,
				Inputs: map[string]any{"username": "new-u", "password": "new-p"},
			},
			Logger:       log,
			HTTP:         httpCtx,
			Secrets:      intCtx.Secrets(),
			Properties:   props,
			Capabilities: capCtx,
		})
		require.NoError(t, err)
		require.NotNil(t, done)
		assert.Equal(t, SetupStepDone, done.Name)
		assert.Contains(t, capCtx.EnabledCapabilities, "hetznerRobot.renameServer")
	})
}

func Test_SetupProvider_OnStepRevert(t *testing.T) {
	s := &SetupProvider{}
	log := logger.DiscardLogger()

	t.Run("capabilitySelection clears capabilities", func(t *testing.T) {
		capCtx := &contexts.CapabilityContext{
			RequestedCapabilties:  []string{"a"},
			AvailableCapabilities: []string{"b"},
		}
		require.NoError(t, s.OnStepRevert(core.SetupStepContext{
			Step:         core.StepInfo{Name: SetupStepCapabilitySelection},
			Logger:       log,
			Capabilities: capCtx,
		}))
		assert.Empty(t, capCtx.RequestedCapabilties)
		assert.Empty(t, capCtx.AvailableCapabilities)
	})

	t.Run("enterCredentials deletes secrets and property", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{NewSetupFlow: true}
		require.NoError(t, intCtx.SetSecret(SecretUsername, []byte("u")))
		require.NoError(t, intCtx.SetSecret(SecretPassword, []byte("p")))
		props := contexts.NewIntegrationPropertyStorage(intCtx)
		require.NoError(t, props.Create(core.IntegrationPropertyDefinition{
			Name: PropertyCredentialPermission, Value: CredentialPermissionReadOnly,
		}))

		require.NoError(t, s.OnStepRevert(core.SetupStepContext{
			Step:       core.StepInfo{Name: SetupStepEnterCredentials},
			Logger:     log,
			Secrets:    intCtx.Secrets(),
			Properties: props,
		}))
		_, uErr := intCtx.Secrets().Get(SecretUsername)
		assert.Error(t, uErr)
		_, pErr := intCtx.Secrets().Get(SecretPassword)
		assert.Error(t, pErr)
		_, propErr := props.GetString(PropertyCredentialPermission)
		assert.Error(t, propErr)
	})

	t.Run("upgradeCredentials moves requested back to available, preserves secrets and enabled capabilities", func(t *testing.T) {
		intCtx := &contexts.IntegrationContext{NewSetupFlow: true}
		require.NoError(t, intCtx.SetSecret(SecretUsername, []byte("u")))
		require.NoError(t, intCtx.SetSecret(SecretPassword, []byte("p")))
		capCtx := &contexts.CapabilityContext{
			EnabledCapabilities:  []string{"hetznerRobot.listServers"},
			RequestedCapabilties: []string{"hetznerRobot.renameServer"},
		}
		require.NoError(t, s.OnStepRevert(core.SetupStepContext{
			Step:         core.StepInfo{Name: SetupStepUpgradeCredentials},
			Logger:       log,
			Secrets:      intCtx.Secrets(),
			Capabilities: capCtx,
		}))
		// Previously-requested capability is now available.
		assert.Empty(t, capCtx.RequestedCapabilties)
		assert.ElementsMatch(t, []string{"hetznerRobot.renameServer"}, capCtx.AvailableCapabilities)
		// Previously-enabled capability is still enabled.
		assert.ElementsMatch(t, []string{"hetznerRobot.listServers"}, capCtx.EnabledCapabilities)
		// Secrets preserved.
		u, err := intCtx.Secrets().Get(SecretUsername)
		require.NoError(t, err)
		assert.Equal(t, "u", u)
	})

	t.Run("unknown step returns error", func(t *testing.T) {
		err := s.OnStepRevert(core.SetupStepContext{
			Step:   core.StepInfo{Name: "bogus"},
			Logger: log,
		})
		require.Error(t, err)
	})
}

func Test_SetupProvider_OnPropertyUpdate(t *testing.T) {
	s := &SetupProvider{}
	_, err := s.OnPropertyUpdate(core.PropertyUpdateContext{
		PropertyName: PropertyCredentialPermission,
		Value:        CredentialPermissionReadOnly,
	})
	require.Error(t, err)
}

func Test_SetupProvider_RequiredCredentialPermission(t *testing.T) {
	t.Run("unknown capability errors", func(t *testing.T) {
		_, err := requiredCredentialPermission([]string{"hetznerRobot.bogus"})
		require.Error(t, err)
	})

	t.Run("only read-only", func(t *testing.T) {
		perm, err := requiredCredentialPermission([]string{"hetznerRobot.listServers", "hetznerRobot.getServer"})
		require.NoError(t, err)
		assert.Equal(t, CredentialPermissionReadOnly, perm)
	})

	t.Run("any mutating capability requires read-write", func(t *testing.T) {
		perm, err := requiredCredentialPermission([]string{"hetznerRobot.listServers", "hetznerRobot.deleteSshKey"})
		require.NoError(t, err)
		assert.Equal(t, CredentialPermissionReadWrite, perm)
	})

	t.Run("empty list defaults to read-only", func(t *testing.T) {
		perm, err := requiredCredentialPermission([]string{})
		require.NoError(t, err)
		assert.Equal(t, CredentialPermissionReadOnly, perm)
	})
}

func Test_SetupProvider_CapabilityPermissionsComplete(t *testing.T) {
	s := &SetupProvider{}
	groups := s.CapabilityGroups()
	names := allCapabilityNames(groups)

	// Every capability in CapabilityGroups must be present in capabilityPermissions.
	// requiredCredentialPermission errors on unknown names — if this passes, the map is complete.
	_, err := requiredCredentialPermission(names)
	require.NoError(t, err, "all capabilities returned by CapabilityGroups() must be present in capabilityPermissions")
}
