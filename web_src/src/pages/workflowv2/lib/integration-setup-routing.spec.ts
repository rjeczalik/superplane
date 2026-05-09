import { describe, expect, it } from "vitest";
import type { IntegrationsIntegrationDefinition, OrganizationsIntegration } from "@/api-client";
import { getCapabilitySetupRoute } from "./integration-setup-routing";

describe("getCapabilitySetupRoute", () => {
  it("routes capability-based integrations to setup with the pending integration id", () => {
    const definition: IntegrationsIntegrationDefinition = {
      name: "vultr",
      legacySetupOnly: false,
    };
    const pendingIntegration: OrganizationsIntegration = {
      metadata: {
        id: "integration-123",
        integrationName: "vultr",
      },
      status: {
        state: "pending",
      },
    };

    expect(
      getCapabilitySetupRoute({
        organizationId: "org-123",
        integrationName: "vultr",
        definition,
        pendingIntegration,
      }),
    ).toEqual({
      path: "/org-123/settings/integrations/vultr/setup",
      state: { integrationId: "integration-123" },
    });
  });

  it("routes capability-based integrations to setup without state when no pending instance exists", () => {
    const definition: IntegrationsIntegrationDefinition = {
      name: "vultr",
      legacySetupOnly: false,
    };

    expect(
      getCapabilitySetupRoute({
        organizationId: "org-123",
        integrationName: "vultr",
        definition,
      }),
    ).toEqual({
      path: "/org-123/settings/integrations/vultr/setup",
      state: undefined,
    });
  });

  it("does not route legacy integrations away from the inline create dialog", () => {
    const definition: IntegrationsIntegrationDefinition = {
      name: "github",
      legacySetupOnly: true,
    };

    expect(
      getCapabilitySetupRoute({
        organizationId: "org-123",
        integrationName: "github",
        definition,
      }),
    ).toBeNull();
  });
});
