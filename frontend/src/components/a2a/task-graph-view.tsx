import { ChevronDown, ChevronRight } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";

export interface TaskGraphNode {
  id: string;
  requester: string;
  delegate: string;
  status: string;
  artifacts: string[];
  approvals: string[];
  budget?: string;
  failureCause?: string;
  children?: TaskGraphNode[];
}

export function TaskGraphView({ nodes }: { nodes: TaskGraphNode[] }) {
  const { t } = useTranslation();
  if (nodes.length === 0) {
    return (
      <p className="rounded-lg border border-control-border p-5 text-sm text-control-light">
        {t("a2a-graph.empty")}
      </p>
    );
  }
  return (
    <ul className="flex flex-col gap-2" aria-label={t("a2a-graph.tree")}>
      {nodes.map((node) => (
        <GraphNode key={node.id} node={node} />
      ))}
    </ul>
  );
}

function GraphNode({ node }: { node: TaskGraphNode }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  const hasChildren = (node.children?.length ?? 0) > 0;
  return (
    <li className="rounded-lg border border-control-border p-3">
      <div className="flex items-start gap-2">
        {hasChildren ? (
          <button
            type="button"
            onClick={() => setOpen((value) => !value)}
            aria-label={open ? t("a2a-graph.collapse") : t("a2a-graph.expand")}
            className="mt-0.5 text-control-light"
          >
            <span aria-hidden="true">
              {open ? (
                <ChevronDown className="size-4" />
              ) : (
                <ChevronRight className="size-4" />
              )}
            </span>
          </button>
        ) : (
          <span className="size-4" />
        )}
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-main">{node.id}</span>
            <Badge variant="secondary">{node.status}</Badge>
          </div>
          <p className="mt-1 text-xs text-control-light">
            {t("a2a-graph.requester")}: {node.requester} ·{" "}
            {t("a2a-graph.delegate")}: {node.delegate}
          </p>
          {node.budget && (
            <p className="mt-1 text-xs text-control-light">
              {t("a2a-graph.budget")}: {node.budget}
            </p>
          )}
          {node.failureCause && (
            <p className="mt-1 text-xs text-destructive">
              {t("a2a-graph.failure")}: {node.failureCause}
            </p>
          )}
          {node.artifacts.length > 0 && (
            <p className="mt-1 text-xs text-control-light">
              {t("a2a-graph.artifacts")}: {node.artifacts.join(", ")}
            </p>
          )}
          {node.approvals.length > 0 && (
            <p className="mt-1 text-xs text-control-light">
              {t("a2a-graph.approvals")}: {node.approvals.join(", ")}
            </p>
          )}
          {open && hasChildren && (
            <ul className="mt-3 ml-3 flex flex-col gap-2 border-l border-control-border pl-3">
              {node.children?.map((child) => (
                <GraphNode key={child.id} node={child} />
              ))}
            </ul>
          )}
        </div>
      </div>
    </li>
  );
}
