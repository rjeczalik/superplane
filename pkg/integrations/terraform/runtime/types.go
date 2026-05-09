package runtime

type DiagSeverity int

const (
	DiagError DiagSeverity = iota
	DiagWarning
)

type ProviderDiagnostic struct {
	Severity  DiagSeverity
	Summary   string
	Detail    string
	Attribute string
}

type CapabilitySchema struct {
	Resources   map[string]ResourceSchema
	DataSources map[string]DataSourceSchema
	Actions     map[string]ActionSchema
}

type DynamicValue struct {
	// JSON is the SuperPlane-facing representation of the Terraform value.
	// Unknown values must use the explicit sentinel shape:
	// {"$terraformUnknown": true, "type": <terraform type json>}.
	JSON []byte
}

type ResourceSchema struct {
	Version    int64
	Attributes []byte
}

type DataSourceSchema struct {
	Attributes []byte
}

type ActionSchema struct {
	Attributes    []byte
	HasPlanStep   bool
	StreamsEvents bool
}

type ActionInput struct {
	Config DynamicValue
}

type ActionOutput struct {
	State       ProviderState
	Result      DynamicValue
	Diagnostics []ProviderDiagnostic
}

type ProviderState struct {
	// Envelope is JSON encoded StateEnvelope. Managed-resource storage encrypts
	// it but never interprets its protocol-specific fields.
	Envelope []byte
}

type StateEnvelope struct {
	FormatVersion int64
	Protocol      string
	TypeName      string
	SchemaVersion int64
	Value         DynamicValue
	Private       []byte
	Identity      []byte
}

type ReplacementMetadata struct {
	RequiresReplace []string
}

type ActionEvent struct {
	Type        string
	Message     string
	Diagnostics []ProviderDiagnostic
}

type ConfigurationField struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Sensitive   bool
	Nested      []ConfigurationField
}
