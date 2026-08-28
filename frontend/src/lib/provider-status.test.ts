import { describe, expect, it } from "vitest";
import {
  providerAction,
  providerAutomaticSelectionDisabled,
  providerDisplayStatus,
} from "./provider-status";

const provider = (overrides: Record<string, unknown> = {}) =>
  ({ providerId: "codex", ...overrides }) as never;

describe("provider status policy", () => {
  it("requires full-loop evidence for automatic readiness", () => {
    expect(
      providerDisplayStatus(
        provider({
          runtimeStatus: "READY",
          compatibilityLevel: "PROTOCOL_READY",
        })
      )
    ).toBe("DETECTED_ONLY");
    expect(
      providerAutomaticSelectionDisabled(
        provider({
          runtimeStatus: "READY",
          compatibilityLevel: "PROTOCOL_READY",
        })
      )
    ).toBe(true);
    expect(
      providerAutomaticSelectionDisabled(
        provider({
          runtimeStatus: "READY",
          compatibilityLevel: "FULL_LOOP_VERIFIED",
        })
      )
    ).toBe(false);
  });

  it("exposes actions only for repairable machine evidence", () => {
    expect(providerAction(provider({ runtimeStatus: "DETECTED" }))).toBe(
      "prepare"
    );
    expect(providerAction(provider({ runtimeStatus: "BROKEN" }))).toBe(
      "repair"
    );
    expect(
      providerAction(provider({ runtimeStatus: "UPDATE_AVAILABLE" }))
    ).toBe("update");
    expect(
      providerAction(provider({ runtimeStatus: "BRIDGE_REQUIRED" }))
    ).toBeNull();
    expect(providerAction(provider({ runtimeStatus: "PULL_ONLY" }))).toBeNull();
    expect(
      providerAction(
        provider({
          runtimeStatus: "READY",
          compatibilityLevel: "PROTOCOL_READY",
        })
      )
    ).toBeNull();
  });
});
