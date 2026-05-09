import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import terraformIcon from "@/assets/terraform-logo.svg";
import { IntegrationIcon } from "./integrationIcons";

describe("IntegrationIcon", () => {
  it("uses a configured Terraform icon before the integration-name logo fallback", () => {
    const { container } = render(<IntegrationIcon integrationName="aws" iconSlug="terraform" />);

    expect(container.querySelector("img")).toHaveAttribute("src", terraformIcon);
  });
});
