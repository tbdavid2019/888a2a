import { Bot } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { providerDisplayStatus } from "@/lib/provider-status";
import type { AgentProviderInfo } from "@/types/proto-es/v1/agent_pb";

export type ProviderCatalogItem = {
  id: string;
  name: string;
  defaultStatus: "BRIDGE_REQUIRED" | "PULL_ONLY" | "PENDING_VERIFICATION";
  transport: string;
};

// Keep this list deliberately metadata-only. A catalog entry is not a promise
// that the runtime is installed or that automatic execution is enabled.
export const PROVIDER_CATALOG: ProviderCatalogItem[] = [
  {
    id: "openclaw",
    name: "OpenClaw",
    defaultStatus: "BRIDGE_REQUIRED",
    transport: "Gateway / CLI",
  },
  {
    id: "hermes",
    name: "Hermes",
    defaultStatus: "BRIDGE_REQUIRED",
    transport: "HTTP / CLI",
  },
  {
    id: "claude-code",
    name: "Claude Code",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "ACP",
  },
  {
    id: "codex",
    name: "Codex",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "ACP v2",
  },
  {
    id: "antigravity",
    name: "Antigravity (agy)",
    defaultStatus: "BRIDGE_REQUIRED",
    transport: "CLI / MCP",
  },
  {
    id: "deepseek-harness",
    name: "DeepSeek Harness",
    defaultStatus: "BRIDGE_REQUIRED",
    transport: "HTTP / CLI",
  },
  {
    id: "workbuddy",
    name: "WorkBuddy",
    defaultStatus: "BRIDGE_REQUIRED",
    transport: "HTTP",
  },
  {
    id: "qwen-office",
    name: "Qwen Office",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "CLI",
  },
  {
    id: "dumate",
    name: "DuMate",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "HTTP",
  },
  {
    id: "traework",
    name: "TraeWork",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "ACP",
  },
  {
    id: "cline",
    name: "Cline",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "ACP",
  },
  {
    id: "zeroclaw",
    name: "ZeroClaw",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "ACP WebSocket",
  },
  {
    id: "qwen-code",
    name: "Qwen Code",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "CLI",
  },
  {
    id: "kiro",
    name: "Kiro CLI",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "CLI",
  },
  {
    id: "github-copilot",
    name: "GitHub Copilot CLI",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "CLI",
  },
  {
    id: "openhands",
    name: "OpenHands",
    defaultStatus: "PULL_ONLY",
    transport: "Pull",
  },
  {
    id: "aider",
    name: "Aider",
    defaultStatus: "PULL_ONLY",
    transport: "CLI / Pull",
  },
  {
    id: "opencode",
    name: "OpenCode",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "ACP",
  },
  {
    id: "goose",
    name: "Goose",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "ACP",
  },
  {
    id: "gemini",
    name: "Gemini CLI",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "CLI",
  },
  {
    id: "cursor",
    name: "Cursor",
    defaultStatus: "PULL_ONLY",
    transport: "Pull",
  },
  {
    id: "grok",
    name: "Grok",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "CLI",
  },
  {
    id: "pi",
    name: "Pi",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "CLI",
  },
  {
    id: "reasonix",
    name: "Reasonix",
    defaultStatus: "PENDING_VERIFICATION",
    transport: "CLI",
  },
];

type ProviderCatalogProps = {
  discoveredProviders: AgentProviderInfo[];
};

export function ProviderCatalog({ discoveredProviders }: ProviderCatalogProps) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<
    "all" | "ready" | "bridge" | "pull" | "pending" | "unavailable"
  >("all");
  const discovered = new Map(
    discoveredProviders.map((provider) => [provider.providerId, provider])
  );
  const filteredCatalog = useMemo(
    () =>
      PROVIDER_CATALOG.filter((item) => {
        const current = discovered.get(item.id);
        const status = current
          ? providerDisplayStatus(current, item.defaultStatus)
          : item.defaultStatus;
        if (filter === "all") return true;
        if (filter === "ready") return status === "READY";
        if (filter === "bridge") return status === "BRIDGE_REQUIRED";
        if (filter === "pull") return status === "PULL_ONLY";
        if (filter === "pending") return status === "PENDING_VERIFICATION";
        return (
          status === "UNAVAILABLE" ||
          status === "BROKEN" ||
          status === "QUARANTINED"
        );
      }),
    [discoveredProviders, filter]
  );

  const filters = [
    ["all", t("machine.provider-catalog-filter-all")],
    ["ready", t("machine.provider-catalog-filter-ready")],
    ["bridge", t("machine.provider-catalog-filter-bridge")],
    ["pull", t("machine.provider-catalog-filter-pull")],
    ["pending", t("machine.provider-catalog-filter-pending")],
    ["unavailable", t("machine.provider-catalog-filter-unavailable")],
  ] as const;

  return (
    <div className="flex flex-col gap-2">
      <div>
        <p className="text-xs font-semibold uppercase tracking-widest text-control-light">
          {t("machine.provider-catalog-title")}
        </p>
        <p className="mt-1 text-xs text-control-light">
          {t("machine.provider-catalog-hint")}
        </p>
      </div>
      <div
        className="flex flex-wrap gap-1"
        role="group"
        aria-label={t("machine.provider-catalog-filters")}
      >
        {filters.map(([value, label]) => (
          <button
            key={value}
            type="button"
            className={`rounded-md px-2 py-1 text-[10px] font-medium ${filter === value ? "bg-control text-background" : "bg-control-subtle text-control"}`}
            onClick={() => setFilter(value)}
          >
            {label}
          </button>
        ))}
      </div>
      <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
        {filteredCatalog.map((item) => {
          const current = discovered.get(item.id);
          const status = current
            ? providerDisplayStatus(current, item.defaultStatus)
            : item.defaultStatus;
          return (
            <li
              key={item.id}
              className="flex min-h-24 flex-col justify-between rounded-lg border border-control-border bg-control-bg/30 p-3"
            >
              <div className="flex items-start gap-2">
                <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-control-subtle text-control">
                  <Bot className="size-4" aria-hidden="true" />
                </span>
                <span className="min-w-0 text-sm font-medium text-main">
                  {current?.displayName || item.name}
                </span>
              </div>
              <div className="mt-2 flex flex-col gap-1">
                <span className="text-[10px] text-control-light">
                  {item.transport}
                </span>
                <span className="text-[10px] font-medium text-control">
                  {status}
                </span>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
