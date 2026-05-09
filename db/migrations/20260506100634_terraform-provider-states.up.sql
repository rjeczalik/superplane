CREATE TABLE terraform_provider_states (
  id                  uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  organization_id     uuid NOT NULL,
  integration_id      uuid NOT NULL REFERENCES app_installations(id) ON DELETE CASCADE,
  canvas_id           uuid NOT NULL,
  node_id             text NOT NULL,
  capability_name     text NOT NULL,
  provider_name       text NOT NULL,
  provider_source     text NOT NULL,
  provider_version    text NOT NULL,
  schema_hash         text NOT NULL,
  state_ciphertext    bytea NOT NULL,
  ad_nonce            bytea NOT NULL,
  lock_version        bigint NOT NULL DEFAULT 0,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  deleted_at          timestamptz
);

CREATE UNIQUE INDEX idx_tf_state_scope
ON terraform_provider_states (organization_id, integration_id, canvas_id, node_id, capability_name, provider_name, provider_source, provider_version)
WHERE deleted_at IS NULL;

CREATE INDEX idx_tf_state_capability_lookup
ON terraform_provider_states (organization_id, integration_id, capability_name, schema_hash)
WHERE deleted_at IS NULL;

CREATE TABLE terraform_provider_state_locks (
  id                  uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  organization_id     uuid NOT NULL,
  integration_id      uuid NOT NULL REFERENCES app_installations(id) ON DELETE CASCADE,
  canvas_id           uuid NOT NULL,
  node_id             text NOT NULL,
  capability_name     text NOT NULL,
  provider_name       text NOT NULL,
  provider_source     text NOT NULL,
  provider_version    text NOT NULL,
  expires_at          timestamptz NOT NULL,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_tf_state_lock_scope
ON terraform_provider_state_locks (organization_id, integration_id, canvas_id, node_id, capability_name, provider_name, provider_source, provider_version);
