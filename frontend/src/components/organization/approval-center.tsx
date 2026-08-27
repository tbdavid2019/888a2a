import { Check, ChevronRight, Clock3, ShieldAlert, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type ApprovalRequestState =
  | "pending"
  | "approved"
  | "denied"
  | "expired"
  | "cancelled"
  | "superseded"
  | "executed";

export type ApprovalDecision = "approve" | "deny";

export interface ApprovalCenterRequest {
  name: string;
  requester: string;
  agent: string;
  actionType: string;
  resource: string;
  destination: string;
  risk: "low" | "moderate" | "high" | "critical";
  parameters: Record<string, string | number | boolean | null>;
  approvalCount: number;
  requiredApprovals: number;
  expiresAt?: string;
  eligible: boolean;
  state: ApprovalRequestState;
}

interface ApprovalCenterProps {
  requests: ApprovalCenterRequest[];
  onDecision: (
    requestName: string,
    decision: ApprovalDecision
  ) => void | Promise<void>;
  canView: boolean;
}

const SENSITIVE_KEY =
  /(token|secret|password|credential|authorization|api.?key|nonce)/i;

function safeParameters(
  parameters: ApprovalCenterRequest["parameters"]
): Array<[string, string]> {
  return Object.entries(parameters)
    .filter(([key]) => !SENSITIVE_KEY.test(key))
    .map(([key, value]) => [key, value === null ? "null" : String(value)]);
}

function riskVariant(
  risk: ApprovalCenterRequest["risk"]
): "default" | "secondary" | "destructive" {
  if (risk === "critical" || risk === "high") return "destructive";
  if (risk === "moderate") return "secondary";
  return "default";
}

function riskLabel(
  t: (key: string) => string,
  risk: ApprovalCenterRequest["risk"]
): string {
  switch (risk) {
    case "critical":
      return t("approval-center.risk-critical");
    case "high":
      return t("approval-center.risk-high");
    case "moderate":
      return t("approval-center.risk-moderate");
    case "low":
      return t("approval-center.risk-low");
  }
}

export function ApprovalCenter({
  requests,
  onDecision,
  canView,
}: ApprovalCenterProps) {
  const { t } = useTranslation();
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const eligibleRequests = useMemo(
    () => requests.filter((request) => request.eligible),
    [requests]
  );
  const selected = eligibleRequests.find(
    (request) => request.name === selectedName
  );

  async function decide(decision: ApprovalDecision) {
    if (!selected || submitting || selected.state !== "pending") return;
    setSubmitting(true);
    try {
      await onDecision(selected.name, decision);
    } finally {
      setSubmitting(false);
    }
  }

  if (!canView) {
    return (
      <p className="p-6 text-sm text-control-light" role="status">
        {t("approval-center.not-allowed")}
      </p>
    );
  }

  return (
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(22rem,0.9fr)]">
      <section
        aria-labelledby="approval-request-list-title"
        className="flex flex-col gap-3"
      >
        <div className="flex items-center justify-between gap-3">
          <h2
            id="approval-request-list-title"
            className="text-sm font-semibold text-main"
          >
            {t("approval-center.pending-title")}
          </h2>
          <Badge variant="secondary">{eligibleRequests.length}</Badge>
        </div>
        {eligibleRequests.length === 0 ? (
          <p className="rounded-lg border border-control-border p-6 text-sm text-control-light">
            {t("approval-center.no-eligible-requests")}
          </p>
        ) : (
          <div className="flex flex-col gap-2" role="list">
            {eligibleRequests.map((request) => (
              <div role="listitem" key={request.name}>
                <button
                  type="button"
                  aria-label={request.resource}
                  onClick={() => setSelectedName(request.name)}
                  className={cn(
                    "flex w-full items-center justify-between gap-4 rounded-lg border p-4 text-left transition-colors hover:bg-accent",
                    selectedName === request.name && "border-control bg-accent"
                  )}
                >
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium text-main">
                      {request.resource}
                    </span>
                    <span className="mt-1 block truncate text-xs text-control-light">
                      {request.actionType} · {request.requester}
                    </span>
                  </span>
                  <span className="flex shrink-0 items-center gap-2">
                    <Badge variant={riskVariant(request.risk)}>
                      {riskLabel(t, request.risk)}
                    </Badge>
                    <ChevronRight
                      className="size-4 text-control-light"
                      aria-hidden="true"
                    />
                  </span>
                </button>
              </div>
            ))}
          </div>
        )}
      </section>

      <section
        aria-labelledby="approval-request-detail-title"
        className="rounded-lg border border-control-border p-5"
      >
        {selected ? (
          <div className="flex flex-col gap-5">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2
                  id="approval-request-detail-title"
                  className="text-base font-semibold text-main"
                >
                  {t("approval-center.intent-title")}
                </h2>
                <p className="mt-1 text-xs text-control-light">
                  {selected.name}
                </p>
              </div>
              <Badge variant={riskVariant(selected.risk)}>
                <ShieldAlert className="mr-1 size-3" aria-hidden="true" />
                {riskLabel(t, selected.risk)}
              </Badge>
            </div>

            <dl className="grid gap-3 text-sm">
              <div>
                <dt className="text-xs text-control-light">
                  {t("approval-center.action")}
                </dt>
                <dd className="font-medium text-main">{selected.actionType}</dd>
              </div>
              <div>
                <dt className="text-xs text-control-light">
                  {t("approval-center.resource")}
                </dt>
                <dd className="break-all font-medium text-main">
                  {selected.resource}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-control-light">
                  {t("approval-center.destination")}
                </dt>
                <dd className="font-medium text-main">
                  {selected.destination}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-control-light">
                  {t("approval-center.requester")}
                </dt>
                <dd className="font-medium text-main">{selected.requester}</dd>
              </div>
              <div>
                <dt className="text-xs text-control-light">
                  {t("approval-center.agent")}
                </dt>
                <dd className="break-all font-medium text-main">
                  {selected.agent}
                </dd>
              </div>
            </dl>

            <div>
              <h3 className="text-xs font-medium text-control-light">
                {t("approval-center.parameters")}
              </h3>
              <dl className="mt-2 grid gap-2 rounded-md bg-accent p-3 text-sm">
                {safeParameters(selected.parameters).map(([key, value]) => (
                  <div className="flex justify-between gap-3" key={key}>
                    <dt className="text-control-light">{key}</dt>
                    <dd className="break-all text-right font-mono text-main">
                      {value}
                    </dd>
                  </div>
                ))}
              </dl>
              <p className="mt-2 text-xs text-control-light">
                {t("approval-center.intent-hash-hidden")}
              </p>
            </div>

            <div className="flex items-center gap-2 text-xs text-control-light">
              <Clock3 className="size-4" aria-hidden="true" />
              <span>
                {t("approval-center.quorum", {
                  approved: selected.approvalCount,
                  required: selected.requiredApprovals,
                })}
              </span>
              {selected.expiresAt && (
                <time dateTime={selected.expiresAt}>{selected.expiresAt}</time>
              )}
            </div>

            <div className="flex flex-wrap gap-2">
              <Button
                disabled={submitting || selected.state !== "pending"}
                onClick={() => void decide("approve")}
              >
                <Check className="size-4" aria-hidden="true" />
                {t("approval-center.approve")}
              </Button>
              <Button
                variant="outline"
                disabled={submitting || selected.state !== "pending"}
                onClick={() => void decide("deny")}
              >
                <X className="size-4" aria-hidden="true" />
                {t("approval-center.deny")}
              </Button>
            </div>
          </div>
        ) : (
          <p className="py-10 text-center text-sm text-control-light">
            {t("approval-center.select-request")}
          </p>
        )}
      </section>
    </div>
  );
}
