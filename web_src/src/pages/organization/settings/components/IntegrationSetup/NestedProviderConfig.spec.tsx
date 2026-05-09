import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { ConfigurationField } from "@/api-client";
import { ConfigurationFieldRenderer } from "@/ui/configurationFieldRenderer";
import { useState } from "react";

const schema: ConfigurationField[] = [
  {
    name: "auth",
    label: "Auth",
    type: "object",
    typeOptions: {
      object: {
        schema: [
          {
            name: "credentials",
            label: "Credentials",
            type: "object",
            typeOptions: {
              object: {
                schema: [
                  { name: "key_id", label: "Key ID", type: "string", required: true },
                  { name: "secret", label: "Secret", type: "string", sensitive: true, required: true },
                ],
              },
            },
          },
          { name: "region", label: "Region", type: "string", required: true },
        ],
      },
    },
  },
  {
    name: "tags",
    label: "Tags",
    type: "list",
    typeOptions: {
      list: {
        itemDefinition: {
          type: "object",
          schema: [
            { name: "key", label: "Key", type: "string", required: true },
            { name: "value", label: "Value", type: "string", required: true },
          ],
        },
      },
    },
  },
];

function FormHarness() {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <div>
      {schema.map((field) => (
        <ConfigurationFieldRenderer
          key={field.name}
          field={field}
          value={values[field.name!]}
          onChange={(value) => setValues((current) => ({ ...current, [field.name!]: value }))}
          allValues={values}
          organizationId="org-1"
        />
      ))}
      <output data-testid="values">{JSON.stringify(values)}</output>
    </div>
  );
}

describe("nested provider config", () => {
  it("renders nested objects, masks sensitive fields, and produces typed values", async () => {
    render(<FormHarness />);

    expect(screen.getByText("Auth")).toBeInTheDocument();
    expect(screen.getByText("Credentials")).toBeInTheDocument();
    expect(screen.getByText("Key ID")).toBeInTheDocument();
    expect(screen.getByText("Secret")).toBeInTheDocument();
    expect(screen.getByText("Region")).toBeInTheDocument();

    await userEvent.type(screen.getByTestId("string-field-key_id"), "kid");
    const secret = screen.getByTestId("string-field-secret");
    expect(secret).toHaveAttribute("type", "password");
    await userEvent.type(secret, "shh");
    await userEvent.type(screen.getByTestId("string-field-region"), "eu-central-1");

    await userEvent.click(screen.getByRole("button", { name: /add item/i }));
    await userEvent.type(screen.getByTestId("string-field-key"), "env");
    await userEvent.type(screen.getByTestId("string-field-value"), "dev");

    expect(JSON.parse(screen.getByTestId("values").textContent || "{}")).toEqual({
      auth: {
        credentials: { key_id: "kid", secret: "shh" },
        region: "eu-central-1",
      },
      tags: [{ key: "env", value: "dev" }],
    });
  });
});
