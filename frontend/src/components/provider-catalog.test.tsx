import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PROVIDER_CATALOG, ProviderCatalog } from "./provider-catalog";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

describe("ProviderCatalog", () => {
  it("renders the complete provider matrix with conservative states", () => {
    render(<ProviderCatalog discoveredProviders={[]} />);

    expect(PROVIDER_CATALOG).toHaveLength(24);
    expect(screen.getByText("OpenClaw")).toBeInTheDocument();
    expect(screen.getByText("Hermes")).toBeInTheDocument();
    expect(screen.getByText("Antigravity (agy)")).toBeInTheDocument();
    expect(screen.getAllByText("BRIDGE_REQUIRED").length).toBeGreaterThan(0);
    expect(screen.getAllByText("PENDING_VERIFICATION").length).toBeGreaterThan(
      0
    );
    expect(screen.getAllByText("PULL_ONLY").length).toBeGreaterThan(0);
  });

  it("uses discovered runtime evidence for a matching provider", () => {
    render(
      <ProviderCatalog
        discoveredProviders={[
          {
            providerId: "openclaw",
            displayName: "OpenClaw Local",
            runtimeStatus: "READY",
          } as never,
        ]}
      />
    );

    expect(screen.getByText("OpenClaw Local")).toBeInTheDocument();
    expect(screen.getByText("READY")).toBeInTheDocument();
  });

  it("filters cards without changing conservative status", () => {
    render(<ProviderCatalog discoveredProviders={[]} />);

    fireEvent.click(
      screen.getByRole("button", {
        name: "machine.provider-catalog-filter-bridge",
      })
    );
    expect(screen.getByText("OpenClaw")).toBeInTheDocument();
    expect(screen.queryByText("OpenHands")).not.toBeInTheDocument();
    expect(screen.getAllByText("BRIDGE_REQUIRED").length).toBeGreaterThan(0);
  });
});
