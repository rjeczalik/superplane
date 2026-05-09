ALTER TABLE terraform_provider_states
  ADD COLUMN IF NOT EXISTS state_format text NOT NULL DEFAULT 'cli-tfstate-v1',
  ADD COLUMN IF NOT EXISTS encryption_version text NOT NULL DEFAULT 'raw-key-ad-v1';

DROP INDEX IF EXISTS idx_tf_state_scope;

CREATE UNIQUE INDEX IF NOT EXISTS idx_tf_state_scope_v2
ON terraform_provider_states (organization_id, integration_id, canvas_id, node_id, capability_name, provider_name, provider_source, provider_version)
WHERE deleted_at IS NULL;
