# Terraform Provider State - Operator Runbook

When a Terraform provider version bump removes or renames a capability that has stored state, server startup fails with a logged error naming:

- provider name
- capability name
- count of affected state rows

## Resolution Paths

### A. Stay On Prior Version

Revert `TERRAFORM_PROVIDER_INTEGRATIONS` env to the prior version. State remains valid; the server will start.

### B. Delete Orphaned State

Connect to the SuperPlane database and run:

```sql
UPDATE terraform_provider_states
SET deleted_at = now()
WHERE provider_name = '<provider-name>'
  AND capability_name = '<capability-name>'
  AND deleted_at IS NULL;
```

Optionally add `AND integration_id = '<integration-id>'` when deleting state for only one installation.

This soft-deletes the rows. Restart the server. Real provider-managed resources are not destroyed by this action; they remain in the cloud and will be unmanaged by SuperPlane.

If the resources should be destroyed before unmanaging:

1. Revert to the prior provider version.
2. Run a canvas using the affected capability with explicit destroy logic. MVP has no `terraform destroy` action.
3. Bump version. Apply path A or B as appropriate.

## Future Admin Command

Post-MVP target: `superplane terraform-state delete --integration=<id> --capability=<name>`, wrapping the SQL above.
