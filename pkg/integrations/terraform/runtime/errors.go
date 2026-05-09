package runtime

import "fmt"

type PluginLifecycleError struct {
	Phase string
	Cause error
}

func (e *PluginLifecycleError) Error() string {
	return fmt.Sprintf("plugin lifecycle error during %s: %v", e.Phase, e.Cause)
}

func (e *PluginLifecycleError) Unwrap() error {
	return e.Cause
}

type RegistryError struct {
	Kind   string
	Detail string
}

func (e *RegistryError) Error() string {
	return fmt.Sprintf("registry error (%s): %s", e.Kind, e.Detail)
}

type ProviderDiagnosticError struct {
	Diagnostics []ProviderDiagnostic
}

func (e *ProviderDiagnosticError) Error() string {
	n := len(e.Diagnostics)
	if n == 0 {
		return "provider returned 0 diagnostic(s)"
	}

	return fmt.Sprintf("provider returned %d diagnostic(s): %s", n, e.Diagnostics[0].Summary)
}

type StateConflictError struct {
	Detail string
}

func (e *StateConflictError) Error() string {
	return fmt.Sprintf("state conflict: %s", e.Detail)
}
