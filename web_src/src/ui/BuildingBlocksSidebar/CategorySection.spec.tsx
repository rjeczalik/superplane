import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import terraformIcon from "@/assets/terraform-logo.svg";
import { CategorySection } from "./CategorySection";
import type { BuildingBlockCategory } from "./types";

function createCategory(name: string): BuildingBlockCategory {
  return {
    name,
    blocks: [
      {
        name: "smtp.send",
        label: "Send Email",
        type: "component",
        integrationName: "smtp",
      },
    ],
  };
}

describe("CategorySection", () => {
  it("does not render the ItemGroup for a non-Core category that is collapsed by default", () => {
    const category = createCategory("Email");

    const { container } = render(
      <CategorySection
        category={category}
        integrations={[]}
        showIntegrationSetupStatus={false}
        canvasZoom={1}
        isDraggingRef={{ current: false }}
        setHoveredBlock={() => {}}
        dragPreviewRef={{ current: null }}
      />,
    );

    expect(screen.getByText("Email")).toBeInTheDocument();
    expect(container.querySelector('[data-slot="item-group"]')).not.toBeInTheDocument();
  });

  it("renders the ItemGroup for the Core category, which is expanded by default", () => {
    const category = createCategory("Core");

    const { container } = render(
      <CategorySection
        category={category}
        integrations={[]}
        showIntegrationSetupStatus={false}
        canvasZoom={1}
        isDraggingRef={{ current: false }}
        setHoveredBlock={() => {}}
        dragPreviewRef={{ current: null }}
      />,
    );

    expect(screen.getByText("Core")).toBeInTheDocument();
    expect(screen.getByText("Send Email")).toBeInTheDocument();
    expect(container.querySelector('[data-slot="item-group"]')).toBeInTheDocument();
  });

  it("renders the ItemGroup for a non-Core category when a search term is present", () => {
    const category = createCategory("Email");

    const { container } = render(
      <CategorySection
        category={category}
        integrations={[]}
        showIntegrationSetupStatus={false}
        canvasZoom={1}
        searchTerm="send"
        isDraggingRef={{ current: false }}
        setHoveredBlock={() => {}}
        dragPreviewRef={{ current: null }}
      />,
    );

    expect(screen.getByText("Email")).toBeInTheDocument();
    expect(screen.getByText("Send Email")).toBeInTheDocument();
    expect(container.querySelector('[data-slot="item-group"]')).toBeInTheDocument();
  });

  it("renders the ItemGroup for a non-Core category after it is manually opened", () => {
    const category = createCategory("Email");

    const { container } = render(
      <CategorySection
        category={category}
        integrations={[]}
        showIntegrationSetupStatus={false}
        canvasZoom={1}
        isDraggingRef={{ current: false }}
        setHoveredBlock={() => {}}
        dragPreviewRef={{ current: null }}
      />,
    );

    const details = container.querySelector("details");
    expect(details).toBeInTheDocument();
    expect(container.querySelector('[data-slot="item-group"]')).not.toBeInTheDocument();

    details!.open = true;
    fireEvent(details!, new Event("toggle"));

    expect(screen.getByText("Send Email")).toBeInTheDocument();
    expect(container.querySelector('[data-slot="item-group"]')).toBeInTheDocument();
  });

  it("uses the configured integration icon for category headers", () => {
    const category = {
      name: "Google Cloud",
      icon: "terraform",
      blocks: [
        {
          name: "google.create_storage_bucket",
          label: "Create Storage Bucket",
          type: "component",
          integrationName: "google",
        },
      ],
    } satisfies BuildingBlockCategory;

    const { container } = render(
      <CategorySection
        category={category}
        integrations={[]}
        showIntegrationSetupStatus={false}
        canvasZoom={1}
        isDraggingRef={{ current: false }}
        setHoveredBlock={() => {}}
        dragPreviewRef={{ current: null }}
      />,
    );

    expect(container.querySelector("summary img")).toHaveAttribute("src", terraformIcon);
  });

  it("uses configured image icons for integration block rows", () => {
    const category = {
      name: "TLS",
      icon: "terraform",
      blocks: [
        {
          name: "terraform_tls.privateKey.create",
          label: "Create Private Key",
          type: "component",
          icon: "terraform",
          integrationName: "terraform_tls",
        },
      ],
    } satisfies BuildingBlockCategory;

    const { container } = render(
      <CategorySection
        category={category}
        integrations={[]}
        showIntegrationSetupStatus={false}
        canvasZoom={1}
        isDraggingRef={{ current: false }}
        setHoveredBlock={() => {}}
        dragPreviewRef={{ current: null }}
      />,
    );

    const details = container.querySelector("details");
    details!.open = true;
    fireEvent(details!, new Event("toggle"));

    const block = screen.getByText("Create Private Key").closest('[data-slot="item"]');
    if (!block) throw new Error("expected rendered building block row");
    expect(block.querySelector("img")).toHaveAttribute("src", terraformIcon);
  });
});
