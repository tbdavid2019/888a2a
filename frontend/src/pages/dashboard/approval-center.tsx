import { useTranslation } from "react-i18next";
import {
  ApprovalCenter,
  type ApprovalCenterRequest,
  type ApprovalDecision,
} from "@/components/organization/approval-center";
import { SettingsPage } from "@/components/settings-page";
import { useHasPermission } from "@/stores/permissions";

export interface ApprovalCenterPageProps {
  requests?: ApprovalCenterRequest[];
  onDecision?: (
    requestName: string,
    decision: ApprovalDecision
  ) => void | Promise<void>;
}

export function ApprovalCenterPage({
  requests = [],
  onDecision = async () => {},
}: ApprovalCenterPageProps) {
  const { t } = useTranslation();
  const canView = useHasPermission("888a2a.settings.get");

  return (
    <SettingsPage
      title={t("approval-center.title")}
      description={t("approval-center.description")}
    >
      <ApprovalCenter
        requests={requests}
        onDecision={onDecision}
        canView={canView}
      />
    </SettingsPage>
  );
}
