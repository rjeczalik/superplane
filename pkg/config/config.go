package config

import (
	"fmt"
	"os"
	"time"
)

func RabbitMQURL() (string, error) {
	URL := os.Getenv("RABBITMQ_URL")
	if URL == "" {
		return "", fmt.Errorf("RABBITMQ_URL not set")
	}

	return URL, nil
}

func UsageGRPCURL() string {
	return os.Getenv("USAGE_GRPC_URL")
}

// AnthropicAgentConfig holds the credentials and identifiers needed to talk
// to a single Anthropic managed agent. Empty values mean managed agents are
// disabled on this installation.
type AnthropicAgentConfig struct {
	APIKey        string
	AgentID       string
	EnvironmentID string
}

// LoadAnthropicAgentConfig reads the env vars for the Anthropic managed-agents
// integration. If any required value is missing, Enabled() returns false.
func LoadAnthropicAgentConfig() AnthropicAgentConfig {
	return AnthropicAgentConfig{
		APIKey:        os.Getenv("ANTHROPIC_API_KEY"),
		AgentID:       os.Getenv("ANTHROPIC_AGENT_ID"),
		EnvironmentID: os.Getenv("ANTHROPIC_ENVIRONMENT_ID"),
	}
}

// Enabled reports whether the Anthropic provider has the credentials it
// needs to run.
func (c AnthropicAgentConfig) Enabled() bool {
	return c.APIKey != "" && c.AgentID != "" && c.EnvironmentID != ""
}

// TerraformProviderCacheDir returns the directory used to cache Terraform
// provider plugins (mapped to TF_PLUGIN_CACHE_DIR for subprocess invocations).
// Default: /var/lib/superplane/tfproviders.
func TerraformProviderCacheDir() string {
	if v := os.Getenv("TERRAFORM_PROVIDER_CACHE_DIR"); v != "" {
		return v
	}
	return "/var/lib/superplane/tfproviders"
}

// TerraformExecutionTimeoutDefault is the upper bound on a single Terraform
// action execution. Effective = min(this, TF resource timeouts.<op>).
// Default: 30m. Parsing errors are fatal at startup.
func TerraformExecutionTimeoutDefault() time.Duration {
	d, err := TerraformExecutionTimeoutDefaultE()
	if err != nil {
		panic(err)
	}
	return d
}

// TerraformExecutionTimeoutDefaultE parses TERRAFORM_EXECUTION_TIMEOUT_DEFAULT
// and returns the duration, or an error if the value is invalid.
func TerraformExecutionTimeoutDefaultE() (time.Duration, error) {
	v := os.Getenv("TERRAFORM_EXECUTION_TIMEOUT_DEFAULT")
	if v == "" {
		return 30 * time.Minute, nil
	}
	return time.ParseDuration(v)
}
