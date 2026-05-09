package runtime

import "fmt"

type Operation string

const (
	OpRead   Operation = "read"
	OpData   Operation = "data"
	OpApply  Operation = "apply"
	OpAction Operation = "action"
)

type ExecutionSpec struct {
	CapabilityName  string
	CapabilityKind  Operation
	ProviderName    string
	ProviderSource  string
	ProviderVersion string
	ResourceName    string
	Operation       Operation
	SchemaHash      string
	SensitiveAttrs  map[string]struct{}
	OutputSchema    []ConfigurationField
	HasPlanStep     bool
}

func (s *ExecutionSpec) Validate() error {
	if s.CapabilityName == "" {
		return fmt.Errorf("execution spec: capability name is required")
	}
	if s.ProviderName == "" {
		return fmt.Errorf("execution spec: provider name is required")
	}
	if s.ProviderSource == "" {
		return fmt.Errorf("execution spec: provider source is required")
	}
	if s.ProviderVersion == "" {
		return fmt.Errorf("execution spec: provider version is required")
	}
	if s.ResourceName == "" {
		return fmt.Errorf("execution spec: resource name is required")
	}
	if s.Operation == "" {
		return fmt.Errorf("execution spec: operation is required")
	}

	return nil
}
