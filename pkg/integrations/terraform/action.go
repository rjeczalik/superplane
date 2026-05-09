package terraform

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

type ActionRunner interface {
	Execute(ctx core.ExecutionContext, action *GeneratedAction) error
}

// GeneratedAction is a core.Action generated from a Terraform provider schema.
type GeneratedAction struct {
	integrationName string
	resourceName    string
	op              string
	description     string
	icon            string
	inputSchema     []configuration.Field
	outputSchema    []configuration.Field
	sensitiveAttrs  map[string]struct{}
	capabilityName  string
	schemaHash      string
	providerName    string
	providerSource  string
	providerVersion string
	capabilityKind  string
	runner          ActionRunner
	hasPlanStep     bool
	streamsEvents   bool
}

// Name returns <integration>.<camelCaseResource>.<op>.
func (a *GeneratedAction) Name() string {
	return fmt.Sprintf("%s.%s.%s", a.integrationName, a.resourceName, a.op)
}

// Label returns a human-readable label.
func (a *GeneratedAction) Label() string {
	words := splitCamelCase(a.resourceName)
	label := strings.Title(strings.Join(words, " "))
	switch a.op {
	case "create":
		return "Create " + label
	case "read":
		return "Get " + label
	case "data":
		return "Get " + label
	default:
		return label
	}
}

// Description returns the action description.
func (a *GeneratedAction) Description() string {
	return a.description
}

// Documentation returns generated markdown documentation.
func (a *GeneratedAction) Documentation() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", a.Label())
	fmt.Fprintf(&b, "Integration: `%s`  \n", a.integrationName)
	fmt.Fprintf(&b, "Resource: `%s`  \n", a.resourceName)
	fmt.Fprintf(&b, "Operation: `%s`  \n\n", a.op)
	if a.description != "" {
		fmt.Fprintf(&b, "%s\n\n", a.description)
	}
	fmt.Fprintf(&b, "## Provider Info\n\n")
	fmt.Fprintf(&b, "- Source: `%s`\n", a.providerSource)
	fmt.Fprintf(&b, "- Version: `%s`\n", a.providerVersion)
	return b.String()
}

func (a *GeneratedAction) Icon() string {
	return a.icon
}

// Color returns an empty string.
func (a *GeneratedAction) Color() string {
	return ""
}

// ExampleOutput returns nil.
func (a *GeneratedAction) ExampleOutput() map[string]any {
	return nil
}

// OutputChannels returns a single default channel.
func (a *GeneratedAction) OutputChannels(configuration any) []core.OutputChannel {
	return []core.OutputChannel{core.DefaultOutputChannel}
}

// Configuration returns the translated input schema.
func (a *GeneratedAction) Configuration() []configuration.Field {
	return a.inputSchema
}

// Setup does nothing.
func (a *GeneratedAction) Setup(ctx core.SetupContext) error {
	return nil
}

func (a *GeneratedAction) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

// Execute runs the Terraform action. In Phase 3 it returns a not-implemented error
// with a temporary defense-in-depth capability guard (r4).
func (a *GeneratedAction) Execute(ctx core.ExecutionContext) error {
	if err := requireCapabilityEnabled(ctx.Integration, a.capabilityName); err != nil {
		return err
	}
	if a.runner == nil {
		return fmt.Errorf("terraform runner not implemented")
	}
	return a.runner.Execute(ctx, a)
}

// Hooks returns nil.
func (a *GeneratedAction) Hooks() []core.Hook {
	return nil
}

// HandleHook does nothing.
func (a *GeneratedAction) HandleHook(ctx core.ActionHookContext) error {
	return nil
}

// HandleWebhook returns zero-effect values.
func (a *GeneratedAction) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return 0, nil, nil
}

// Cancel does nothing.
func (a *GeneratedAction) Cancel(ctx core.ExecutionContext) error {
	return nil
}

// Cleanup does nothing.
func (a *GeneratedAction) Cleanup(ctx core.SetupContext) error {
	return nil
}

// CapabilityName returns the fully-qualified capability name.
func (a *GeneratedAction) CapabilityName() string {
	return a.capabilityName
}

// SchemaHash returns the SHA256 hash of the canonical source schema.
func (a *GeneratedAction) SchemaHash() string {
	return a.schemaHash
}

// ProviderName returns the provider short name.
func (a *GeneratedAction) ProviderName() string {
	return a.providerName
}

// ProviderSource returns the provider source address.
func (a *GeneratedAction) ProviderSource() string {
	return a.providerSource
}

// ProviderVersion returns the provider version.
func (a *GeneratedAction) ProviderVersion() string {
	return a.providerVersion
}

// splitCamelCase splits a CamelCase string into words.
func splitCamelCase(s string) []string {
	var words []string
	var current []rune
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			words = append(words, string(current))
			current = nil
		}
		current = append(current, r)
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}
