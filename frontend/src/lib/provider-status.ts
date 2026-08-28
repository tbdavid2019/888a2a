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
  return [
    "QUARANTINED",
    "BROKEN",
    "DETECTED",
    "DETECTED_ONLY",
    "UPDATE_AVAILABLE",
  ].includes(provider.runtimeStatus || "DETECTED");
}

export function providerAutomaticSelectionDisabled(
  provider: AgentProviderInfo
): boolean {
  return !providerIsAutomaticReady(provider);
}
