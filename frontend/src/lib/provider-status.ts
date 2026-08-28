import type { AgentProviderInfo } from "@/types/proto-es/v1/agent_pb";

// Detection and protocol readiness are not enough to allow automatic A2A
// execution. The machine must report a completed full-loop verification.
export function providerIsAutomaticReady(provider: AgentProviderInfo): boolean {
  return (
    provider.runtimeStatus === "READY" &&
    provider.compatibilityLevel === "FULL_LOOP_VERIFIED"
  );
}

export function providerDisplayStatus(
  provider: AgentProviderInfo,
  fallback = "DETECTED"
): string {
  if (providerIsAutomaticReady(provider)) return "READY";
  if (
    provider.runtimeStatus === "READY" ||
    provider.runtimeStatus === "DETECTED"
  ) {
    return "DETECTED_ONLY";
  }
  return provider.runtimeStatus || fallback;
}

export function providerNeedsPreparation(provider: AgentProviderInfo): boolean {
  return providerAction(provider) !== null;
}

export type ProviderAction = "prepare" | "repair" | "update";

// These are the only automatic Machine actions. Bridge-required, pull-only,
// pending, and unverified providers intentionally return null.
export function providerAction(
  provider: AgentProviderInfo
): ProviderAction | null {
  switch (provider.runtimeStatus) {
    case "QUARANTINED":
    case "BROKEN":
      return "repair";
    case "DETECTED":
      return "prepare";
    case "UPDATE_AVAILABLE":
      return "update";
    default:
      return null;
  }
}

export function providerAutomaticSelectionDisabled(
  provider: AgentProviderInfo
): boolean {
  return !providerIsAutomaticReady(provider);
}
