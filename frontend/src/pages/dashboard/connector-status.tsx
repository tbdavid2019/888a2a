import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ConnectorStatus } from "@/components/organization/connector-status";
import { PermissionNotice, SettingsPage } from "@/components/settings-page";
import { connectorServiceClient } from "@/connect";
import { describeError } from "@/lib/connect-errors";
import { toastManager } from "@/lib/toast";
import { useAppStore } from "@/stores";
import { useHasPermission } from "@/stores/permissions";
import type { ConnectorInstallation } from "@/types/proto-es/a2a888/connector_pb";

export function ConnectorStatusPage() {
  const { t } = useTranslation();
  const canView = useHasPermission("888a2a.settings.get");
  const organizationID = useAppStore((state) => state.currentOrganizationId);
  const [installations, setInstallations] = useState<ConnectorInstallation[]>(
    []
  );
  const [accessDenied, setAccessDenied] = useState(false);
  useEffect(() => {
    if (!canView) return;
    let active = true;
    void connectorServiceClient
      .listConnectorInstallations({ organizationId: organizationID })
      .then((response) => {
        if (active) setInstallations(response.installations);
      })
      .catch((error: unknown) => {
        if (active) {
          setAccessDenied(true);
          toastManager.add({
            type: "error",
            title: t("connector-status.load-failed"),
            description: describeError(error),
          });
        }
      });
    return () => {
      active = false;
    };
  }, [canView, organizationID, t]);
  return (
    <SettingsPage
      title={t("connector-status.title")}
      description={t("connector-status.description")}
    >
      {!canView || accessDenied ? (
        <PermissionNotice message={t("connector-status.not-allowed")} />
      ) : (
        <ConnectorStatus installations={installations} />
      )}
    </SettingsPage>
  );
}
