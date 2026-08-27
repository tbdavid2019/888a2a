import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { UsageSummary } from "./usage-summary";

describe("UsageSummary", () => {
  it("shows read-only state and hides nothing beyond aggregate data", () => {
    render(
      <UsageSummary
        summary={
          {
            aggregates: [
              {
                name: "usage/1",
                feature: "agent.turn",
                unit: "count",
                quantity: 2,
              },
            ],
            entitlements: [
              {
                name: "entitlement/1",
                feature: "agent.turn",
                enabled: true,
                limit: 10,
                unit: "count",
              },
            ],
            readOnly: true,
          } as never
        }
      />
    );
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.getAllByText("agent.turn")).toHaveLength(2);
    expect(screen.getByText(/2 count/)).toBeInTheDocument();
  });
});
