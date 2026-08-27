import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ConnectorStatus } from "./connector-status";

describe("ConnectorStatus", () => {
  it("shows health and durable delivery counters", () => {
    render(
      <ConnectorStatus
        installations={
          [
            {
              name: "organizations/org-a/connectorInstallations/line-a",
              installationId: "line-a",
              kind: "line",
              health: 1,
              pendingDeliveries: 2n,
              deadLetterDeliveries: 1n,
            },
          ] as never
        }
      />
    );
    expect(screen.getByText("line-a")).toBeInTheDocument();
    expect(screen.getByText("connector-status.pending")).toBeInTheDocument();
    expect(
      screen.getByText("connector-status.dead-letter")
    ).toBeInTheDocument();
  });

  it("renders an empty state", () => {
    render(<ConnectorStatus installations={[]} />);
    expect(screen.getByText("connector-status.empty")).toBeInTheDocument();
  });
});
