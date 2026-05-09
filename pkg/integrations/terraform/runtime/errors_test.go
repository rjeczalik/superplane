package runtime_test

import (
	"errors"
	"testing"

	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
)

func TestPluginLifecycleError(t *testing.T) {
	tests := []struct {
		name string
		err  *runtime.PluginLifecycleError
		msg  string
	}{
		{
			name: "launch failure",
			err:  &runtime.PluginLifecycleError{Phase: "launch", Cause: errors.New("binary not found")},
			msg:  "plugin lifecycle error during launch: binary not found",
		},
		{
			name: "crash",
			err:  &runtime.PluginLifecycleError{Phase: "execute", Cause: errors.New("signal: killed")},
			msg:  "plugin lifecycle error during execute: signal: killed",
		},
		{
			name: "timeout",
			err:  &runtime.PluginLifecycleError{Phase: "handshake", Cause: errors.New("context deadline exceeded")},
			msg:  "plugin lifecycle error during handshake: context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("got %q, want %q", tt.err.Error(), tt.msg)
			}

			var target *runtime.PluginLifecycleError
			if !errors.As(tt.err, &target) {
				t.Error("errors.As failed for PluginLifecycleError")
			}
		})
	}
}

func TestRegistryError(t *testing.T) {
	err := &runtime.RegistryError{Kind: "checksum", Detail: "sha256 mismatch"}
	if err.Error() != "registry error (checksum): sha256 mismatch" {
		t.Errorf("got %q", err.Error())
	}

	var target *runtime.RegistryError
	if !errors.As(err, &target) {
		t.Error("errors.As failed for RegistryError")
	}
}

func TestProviderDiagnosticError(t *testing.T) {
	err := &runtime.ProviderDiagnosticError{
		Diagnostics: []runtime.ProviderDiagnostic{
			{Severity: runtime.DiagError, Summary: "invalid config"},
		},
	}
	if err.Error() != "provider returned 1 diagnostic(s): invalid config" {
		t.Errorf("got %q", err.Error())
	}
}

func TestStateConflictError(t *testing.T) {
	err := &runtime.StateConflictError{Detail: "lock version mismatch"}
	if err.Error() != "state conflict: lock version mismatch" {
		t.Errorf("got %q", err.Error())
	}
}

func TestErrorCategoriesAreDistinguishable(t *testing.T) {
	errs := []error{
		&runtime.PluginLifecycleError{Phase: "launch", Cause: errors.New("x")},
		&runtime.RegistryError{Kind: "resolution", Detail: "x"},
		&runtime.ProviderDiagnosticError{},
		&runtime.StateConflictError{Detail: "x"},
	}

	for i, err := range errs {
		for j, other := range errs {
			if i == j {
				continue
			}

			switch other.(type) {
			case *runtime.PluginLifecycleError:
				var target *runtime.PluginLifecycleError
				if errors.As(err, &target) {
					t.Errorf("errs[%d] unexpectedly matched PluginLifecycleError", i)
				}
			case *runtime.RegistryError:
				var target *runtime.RegistryError
				if errors.As(err, &target) {
					t.Errorf("errs[%d] unexpectedly matched RegistryError", i)
				}
			case *runtime.ProviderDiagnosticError:
				var target *runtime.ProviderDiagnosticError
				if errors.As(err, &target) {
					t.Errorf("errs[%d] unexpectedly matched ProviderDiagnosticError", i)
				}
			case *runtime.StateConflictError:
				var target *runtime.StateConflictError
				if errors.As(err, &target) {
					t.Errorf("errs[%d] unexpectedly matched StateConflictError", i)
				}
			}
		}
	}
}
