DROP INDEX IF EXISTS idx_tf_state_scope_v2;

CREATE UNIQUE INDEX IF NOT EXISTS idx_tf_state_scope
ON terraform_provider_states (organization_id, integration_id, canvas_id, node_id, capability_name, provider_name, provider_source, provider_version)
WHERE deleted_at IS NULL;

ALTER TABLE terraform_provider_states
  DROP COLUMN IF EXISTS encryption_version,
  DROP COLUMN IF EXISTS state_format;
