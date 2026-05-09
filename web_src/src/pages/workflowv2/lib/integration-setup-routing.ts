import type { IntegrationsIntegrationDefinition, OrganizationsIntegration } from "@/api-client";
import { isCapabilityBasedIntegrationDefinition } from "@/lib/integrations";

export type CapabilitySetupRoute = {
  path: string;
  state?: { integrationId: string };
};

export function getCapabilitySetupRoute({
  organizationId,
  integrationName,
  definition,
  pendingIntegration,
}: {
  organizationId: string | undefined;
  integrationName: string;
  definition: IntegrationsIntegrationDefinition | undefined;
  pendingIntegration?: OrganizationsIntegration;
}): CapabilitySetupRoute | null {
  if (!organizationId || !definition || !isCapabilityBasedIntegrationDefinition(definition)) {
    return null;
  }

  const integrationId = pendingIntegration?.metadata?.id;
  return {
    path: `/${organizationId}/settings/integrations/${integrationName}/setup`,
    state: integrationId ? { integrationId } : undefined,
  };
}
