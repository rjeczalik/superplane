BEGIN;

DROP TABLE IF EXISTS terraform_managed_resource_subscription_cursors;
DROP TABLE IF EXISTS terraform_managed_resource_subscriptions;
DROP TABLE IF EXISTS terraform_managed_resource_events;
DROP TABLE IF EXISTS terraform_managed_resource_states;
DROP TABLE IF EXISTS terraform_managed_resources;

COMMIT;
