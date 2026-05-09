import type { IntegrationsIntegrationDefinition } from "@/api-client";
import { describe, expect, it } from "vitest";
import { buildBuildingBlockCategories } from "./buildingBlocks";

describe("buildBuildingBlockCategories", () => {
  it("orders categories as Core, Debugging, then Memory", () => {
    const categories = buildBuildingBlockCategories(
      [],
      [
        { name: "deploy", label: "Deploy" },
        { name: "display", label: "Display" },
        { name: "addmemory", label: "Add Memory" },
      ],
      [],
    );

    expect(categories.map((category) => category.name)).toEqual(["Core", "Debugging", "Memory"]);
  });

  it("preserves integration definition icons on integration categories", () => {
    const integration: IntegrationsIntegrationDefinition = {
      name: "google",
      label: "Google Cloud",
      icon: "terraform",
      capabilities: [
        {
          type: "TYPE_ACTION",
          name: "google.create_storage_bucket",
          label: "Create Storage Bucket",
        },
      ],
    };

    const categories = buildBuildingBlockCategories([], [], [integration]);

    expect(categories[0]?.icon).toBe("terraform");
  });

  it("applies integration definition icons to generated integration blocks", () => {
    const integration: IntegrationsIntegrationDefinition = {
      name: "terraform_tls",
      label: "TLS",
      icon: "terraform",
      capabilities: [
        {
          type: "TYPE_TRIGGER",
          name: "terraform_tls.privateKey.onChanged",
          label: "On Private Key Changed",
        },
        {
          type: "TYPE_ACTION",
          name: "terraform_tls.privateKey.create",
          label: "Create Private Key",
        },
      ],
    };

    const categories = buildBuildingBlockCategories([], [], [integration]);

    expect(categories[0]?.blocks.map((block) => block.icon)).toEqual(["terraform", "terraform"]);
  });

  it("uses integration names as stable category ids when labels collide", () => {
    const categories = buildBuildingBlockCategories(
      [],
      [],
      [
        {
          name: "aws",
          label: "AWS",
          capabilities: [{ type: "TYPE_ACTION", name: "aws.ec2.createImage", label: "Create Image" }],
        },
        {
          name: "terraform_aws",
          label: "AWS",
          capabilities: [{ type: "TYPE_ACTION", name: "terraform_aws.instance.create", label: "Create Instance" }],
        },
      ],
    );

    expect(categories.map((category) => category.name)).toEqual(["AWS", "AWS"]);
    expect(categories.map((category) => category.id)).toEqual(["aws", "terraform_aws"]);
  });
});
