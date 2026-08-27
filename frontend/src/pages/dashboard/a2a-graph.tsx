import { useTranslation } from "react-i18next";
import {
  type TaskGraphNode,
  TaskGraphView,
} from "@/components/a2a/task-graph-view";
import { SettingsPage } from "@/components/settings-page";

export function A2AGraphPage({ nodes = [] }: { nodes?: TaskGraphNode[] }) {
  const { t } = useTranslation();
  return (
    <SettingsPage
      title={t("a2a-graph.title")}
      description={t("a2a-graph.description")}
    >
      <TaskGraphView nodes={nodes} />
    </SettingsPage>
  );
}
