import { Code, ConnectError } from "@connectrpc/connect";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { UsageSummary } from "@/components/organization/usage-summary";
import { PermissionNotice, SettingsPage } from "@/components/settings-page";
import { usageServiceClient } from "@/connect";
import { describeError } from "@/lib/connect-errors";
import { toastManager } from "@/lib/toast";
import { useAppStore } from "@/stores";
import { useHasPermission } from "@/stores/permissions";
import type { GetUsageSummaryResponse } from "@/types/proto-es/a2a888/usage_pb";

export function UsageSummaryPage() {
  const { t } = useTranslation();
  const canView = useHasPermission("888a2a.settings.get");
  const organizationID = useAppStore((state) => state.currentOrganizationId);
  const [summary, setSummary] = useState<GetUsageSummaryResponse | null>(null);
  const [accessDenied, setAccessDenied] = useState(false);

  useEffect(() => {
    if (!canView) return;
    let active = true;
    void usageServiceClient
      .getUsageSummary({ organizationId: organizationID })
      .then((response) => {
        if (active) setSummary(response);
      })
      .catch((error: unknown) => {
        if (active) {
          if (
            error instanceof ConnectError &&
            error.code === Code.PermissionDenied
          ) {
            setAccessDenied(true);
          } else {
            toastManager.add({
              type: "error",
              title: t("usage-summary.load-failed"),
              description: describeError(error),
            });
          }
        }
      });
    return () => {
      active = false;
    };
  }, [canView, organizationID, t]);

  return (
    <SettingsPage
      title={t("usage-summary.title")}
      description={t("usage-summary.description")}
    >
      {!canView || accessDenied ? (
        <PermissionNotice message={t("usage-summary.not-allowed")} />
      ) : summary ? (
        <UsageSummary summary={summary} />
      ) : (
        <p className="text-sm text-control-light" role="status">
          {t("usage-summary.loading")}
        </p>
      )}
    </SettingsPage>
  );
}
