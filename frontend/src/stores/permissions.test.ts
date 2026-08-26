import { describe, expect, it } from "vitest";
import { useAppStore } from "./index";
import { hasPermission } from "./permissions";

describe("permission namespace compatibility", () => {
  it("accepts a legacy permission when the requested namespace is new", () => {
    useAppStore.setState({
      currentUser: { permissions: ["laelia.agents.edit"] } as never,
    });
    expect(hasPermission("888a2a.agents.edit")).toBe(true);
  });

  it("accepts a new permission when the requested namespace is legacy", () => {
    useAppStore.setState({
      currentUser: { permissions: ["888a2a.agents.edit"] } as never,
    });
    expect(hasPermission("laelia.agents.edit")).toBe(true);
  });
});
