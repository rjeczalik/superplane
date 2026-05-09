import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/ui/tooltip";
import { IntegrationOriginBadge } from "./IntegrationOriginBadge";

function renderBadge(props: React.ComponentProps<typeof IntegrationOriginBadge>) {
  return render(
    <TooltipProvider>
      <IntegrationOriginBadge {...props} />
    </TooltipProvider>,
  );
}

describe("IntegrationOriginBadge", () => {
  it("renders nothing for native integrations", () => {
    const { container } = renderBadge({ origin: "native" });
    expect(container).toBeEmptyDOMElement();
  });

  it("renders terraform badge", () => {
    renderBadge({ origin: "terraform", source: "registry.terraform.io/siderolabs/talos", version: "0.11.0" });
    expect(screen.getByText("Terraform")).toBeInTheDocument();
  });

  it("shows source and version in tooltip", async () => {
    renderBadge({ origin: "terraform", source: "registry.terraform.io/siderolabs/talos", version: "0.11.0" });
    await userEvent.hover(screen.getByText("Terraform"));
    expect(await screen.findAllByText("registry.terraform.io/siderolabs/talos @ 0.11.0")).not.toHaveLength(0);
  });
});
