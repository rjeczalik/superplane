CREATE TABLE IF NOT EXISTS terraform_provider_state_locks (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  organization_id uuid NOT NULL,
  integration_id uuid NOT NULL REFERENCES app_installations(id) ON DELETE CASCADE,
  canvas_id uuid NOT NULL,
  node_id text NOT NULL,
  capability_name text NOT NULL,
  provider_name text NOT NULL,
  provider_source text NOT NULL,
  provider_version text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tf_state_lock_scope
ON terraform_provider_state_locks (organization_id, integration_id, canvas_id, node_id, capability_name, provider_name, provider_source, provider_version);
