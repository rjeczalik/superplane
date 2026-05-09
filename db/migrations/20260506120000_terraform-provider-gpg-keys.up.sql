CREATE TABLE terraform_provider_gpg_keys (
  id              uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  provider_source text NOT NULL,
  key_id          text NOT NULL,
  fingerprint     text NOT NULL,
  ascii_armor     text NOT NULL,
  trust_mode      text NOT NULL,
  pinned_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_tf_provider_gpg_keys_source_fingerprint
ON terraform_provider_gpg_keys (provider_source, fingerprint);

CREATE UNIQUE INDEX idx_tf_provider_gpg_keys_source
ON terraform_provider_gpg_keys (provider_source);
