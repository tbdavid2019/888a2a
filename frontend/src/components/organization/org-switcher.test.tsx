import { create } from "@bufbuild/protobuf";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import { useAppStore } from "@/stores";
import {
  OrganizationSchema,
  OrganizationState,
} from "@/types/proto-es/a2a888/organization_pb";
import { OrgSwitcher } from "./org-switcher";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

describe("OrgSwitcher", () => {
  afterEach(() => {
    document.body.innerHTML = "";
    useAppStore.setState({
      currentOrganizationId: "default",
      organizations: [],
    });
  });

  it("renders with default organization when empty", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(async () => {
      root.render(<OrgSwitcher />);
    });

    const trigger = container.querySelector(
      '[data-testid="org-switcher-trigger"]'
    );
    expect(trigger).not.toBeNull();
    expect(trigger?.textContent).toContain("Default Organization");

    await act(async () => {
      root.unmount();
    });
  });

  it("shows suspended badge when current organization is suspended", async () => {
    const suspendedOrg = create(OrganizationSchema, {
      id: "org-suspended",
      name: "Suspended Org",
      slug: "suspended-org",
      state: OrganizationState.SUSPENDED,
    });

    useAppStore.setState({
      currentOrganizationId: "org-suspended",
      organizations: [suspendedOrg],
    });

    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(async () => {
      root.render(<OrgSwitcher />);
    });

    const badge = container.querySelector(
      '[data-testid="org-suspended-badge"]'
    );
    expect(badge).not.toBeNull();
    expect(badge?.textContent).toContain("Tenant Suspended (Read-Only)");

    await act(async () => {
      root.unmount();
    });
  });
});
