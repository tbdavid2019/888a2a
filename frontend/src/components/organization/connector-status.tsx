import { AlertTriangle, CheckCircle2, XCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { ConnectorInstallation } from "@/types/proto-es/a2a888/connector_pb";

export function ConnectorStatus({
  installations,
}: {
  installations: ConnectorInstallation[];
}) {
  const { t } = useTranslation();
  if (installations.length === 0) {
    return (
      <p className="rounded-lg border border-control-border p-5 text-sm text-control-light">
        {t("connector-status.empty")}
      </p>
    );
  }
  return (
    <ul className="divide-y divide-control-border rounded-lg border border-control-border">
      {installations.map((installation) => {
        const health = String(installation.health);
        const failed = health.includes("FAILED");
        const degraded = health.includes("DEGRADED");
        const Icon = failed ? XCircle : degraded ? AlertTriangle : CheckCircle2;
        return (
          <li
            key={installation.name}
            className="flex flex-wrap items-center justify-between gap-4 p-4"
          >
            <div className="flex min-w-0 items-center gap-3">
              <Icon className="size-4 text-control-light" aria-hidden="true" />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-main">
                  {installation.installationId}
                </p>
                <p className="text-xs text-control-light">
                  {installation.kind}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2 text-xs text-control-light">
              <Badge
                variant={
                  failed ? "destructive" : degraded ? "secondary" : "default"
                }
              >
                {health}
              </Badge>
              <span>
                {t("connector-status.pending", {
                  count: installation.pendingDeliveries,
                })}
              </span>
              <span>
                {t("connector-status.dead-letter", {
                  count: installation.deadLetterDeliveries,
                })}
              </span>
            </div>
          </li>
        );
      })}
    </ul>
  );
}
