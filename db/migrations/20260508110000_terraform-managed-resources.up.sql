BEGIN;

CREATE TABLE terraform_managed_resources (
  id                      uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  managed_resource_id     uuid NOT NULL UNIQUE DEFAULT uuid_generate_v4(),
  organization_id         uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
  integration_id          uuid NOT NULL REFERENCES app_installations(id) ON DELETE RESTRICT,
  canvas_id               uuid NOT NULL REFERENCES workflows(id) ON DELETE RESTRICT,
  created_by_node_id      text,
  created_by_execution_id uuid REFERENCES workflow_node_executions(id) ON DELETE SET NULL,
  created_by_event_id     uuid REFERENCES workflow_events(id) ON DELETE SET NULL,
  root_event_id           uuid REFERENCES workflow_events(id) ON DELETE SET NULL,
  provider_name           text NOT NULL,
  provider_source         text NOT NULL,
  provider_version        text NOT NULL,
  resource_type           text NOT NULL,
  idempotency_key         text,
  remote_id               text,
  display_name            text,
  status                  text NOT NULL,
  health                  text NOT NULL,
  last_operation          text,
  retention_policy        jsonb NOT NULL DEFAULT '{}'::jsonb,
  recovery_hints          jsonb NOT NULL DEFAULT '{}'::jsonb,
  last_refreshed_at       timestamptz,
  missing_count           integer NOT NULL DEFAULT 0,
  error_count             integer NOT NULL DEFAULT 0,
  orphan_risk             boolean NOT NULL DEFAULT false,
  last_error              text,
  last_error_at           timestamptz,
  current_operation_id    uuid,
  operation_started_at    timestamptz,
  operation_expires_at    timestamptz,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now(),
  deleted_at              timestamptz,

  CONSTRAINT chk_tmr_status CHECK (status IN ('creating', 'ready', 'updating', 'deleting', 'missing', 'deleted', 'deleted_external')),
  CONSTRAINT chk_tmr_health CHECK (health IN ('healthy', 'degraded', 'unreachable'))
);

CREATE UNIQUE INDEX idx_tmr_idempotency
ON terraform_managed_resources (canvas_id, integration_id, resource_type, idempotency_key)
WHERE idempotency_key IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX idx_tmr_canvas_integration_resource_status
ON terraform_managed_resources (canvas_id, integration_id, resource_type, status);

CREATE INDEX idx_tmr_organization_integration
ON terraform_managed_resources (organization_id, integration_id);

CREATE INDEX idx_tmr_created_by_execution_id
ON terraform_managed_resources (created_by_execution_id);

CREATE INDEX idx_tmr_poll_schedule
ON terraform_managed_resources (status, last_refreshed_at)
WHERE deleted_at IS NULL;

CREATE INDEX idx_tmr_operation_lease
ON terraform_managed_resources (current_operation_id, operation_expires_at)
WHERE current_operation_id IS NOT NULL;

CREATE TABLE terraform_managed_resource_states (
  id                         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  managed_resource_id        uuid NOT NULL UNIQUE REFERENCES terraform_managed_resources(managed_resource_id) ON DELETE CASCADE,
  state_ciphertext           bytea NOT NULL,
  state_nonce                bytea NOT NULL,
  last_config_ciphertext     bytea NOT NULL,
  last_config_nonce          bytea NOT NULL,
  schema_hash                text NOT NULL,
  encryption_version         integer NOT NULL DEFAULT 1,
  state_format               text NOT NULL,
  lock_version               bigint NOT NULL DEFAULT 0,
  created_at                 timestamptz NOT NULL DEFAULT now(),
  updated_at                 timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE terraform_managed_resource_events (
  id                  uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  managed_resource_id uuid NOT NULL REFERENCES terraform_managed_resources(managed_resource_id) ON DELETE CASCADE,
  organization_id     uuid NOT NULL,
  canvas_id           uuid NOT NULL,
  integration_id      uuid NOT NULL,
  resource_type       text NOT NULL,
  event_type          text NOT NULL,
  state               text NOT NULL DEFAULT 'pending',
  outputs_hash        text,
  outputs             jsonb NOT NULL DEFAULT '{}'::jsonb,
  hash_input          jsonb NOT NULL DEFAULT '{}'::jsonb,
  metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
  dispatch_attempts   integer NOT NULL DEFAULT 0,
  last_dispatch_error text,
  processed_at        timestamptz,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT chk_tmre_state CHECK (state IN ('pending', 'processed')),
  CONSTRAINT chk_tmre_event_type CHECK (event_type IN (
    'resource_created',
    'resource_updated',
    'resource_replaced',
    'resource_deleted',
    'resource_forgotten',
    'resource_missing',
    'resource_externally_deleted',
    'resource_recovered'
  ))
);

CREATE INDEX idx_tmre_pending
ON terraform_managed_resource_events (state, created_at)
WHERE state = 'pending';

CREATE INDEX idx_tmre_resource_created_at
ON terraform_managed_resource_events (managed_resource_id, created_at);

CREATE TABLE terraform_managed_resource_subscriptions (
  id                  uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  organization_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
  canvas_id           uuid NOT NULL REFERENCES workflows(id) ON DELETE RESTRICT,
  integration_id      uuid NOT NULL REFERENCES app_installations(id) ON DELETE RESTRICT,
  node_id             text NOT NULL,
  resource_type       text NOT NULL,
  managed_resource_id uuid REFERENCES terraform_managed_resources(managed_resource_id) ON DELETE CASCADE,
  idempotency_key     text,
  changed_fields      jsonb NOT NULL DEFAULT '[]'::jsonb,
  poll_interval_secs  integer NOT NULL DEFAULT 300,
  backoff_secs        integer NOT NULL DEFAULT 0,
  last_poll_at        timestamptz,
  enabled             boolean NOT NULL DEFAULT true,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  deleted_at          timestamptz,

  CONSTRAINT fk_tmrs_workflow_node
    FOREIGN KEY (canvas_id, node_id) REFERENCES workflow_nodes(workflow_id, node_id),
  CONSTRAINT chk_tmrs_poll_interval_positive CHECK (poll_interval_secs > 0),
  CONSTRAINT chk_tmrs_backoff_nonnegative CHECK (backoff_secs >= 0)
);

CREATE INDEX idx_tmrs_canvas_integration_resource
ON terraform_managed_resource_subscriptions (canvas_id, integration_id, resource_type);

CREATE INDEX idx_tmrs_managed_resource
ON terraform_managed_resource_subscriptions (managed_resource_id)
WHERE managed_resource_id IS NOT NULL;

CREATE UNIQUE INDEX idx_tmrs_node_resource_unique
ON terraform_managed_resource_subscriptions (node_id, canvas_id, integration_id, resource_type)
WHERE deleted_at IS NULL;

CREATE TABLE terraform_managed_resource_subscription_cursors (
  id                  uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  subscription_id     uuid NOT NULL REFERENCES terraform_managed_resource_subscriptions(id) ON DELETE CASCADE,
  managed_resource_id uuid NOT NULL REFERENCES terraform_managed_resources(managed_resource_id) ON DELETE CASCADE,
  last_outputs_hash   text,
  last_event_id       uuid REFERENCES terraform_managed_resource_events(id) ON DELETE SET NULL,
  last_emitted_at     timestamptz,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_tmrs_cursor_subscription_resource
ON terraform_managed_resource_subscription_cursors (subscription_id, managed_resource_id);

COMMIT;
