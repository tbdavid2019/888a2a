import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { type GetUsageSummaryResponse } from "@/types/proto-es/a2a888/usage_pb";

interface UsageSummaryProps {
  summary: GetUsageSummaryResponse;
}

export function UsageSummary({ summary }: UsageSummaryProps) {
  const { t } = useTranslation();
  const subscriptionState = summary.subscription?.state;

  return (
    <div className="flex flex-col gap-5">
      {summary.readOnly && (
        <p
          className="rounded-lg border border-warning/40 bg-warning/10 p-4 text-sm text-main"
          role="status"
        >
          {t("usage-summary.read-only")}
        </p>
      )}
      <section
        aria-labelledby="usage-summary-entitlements"
        className="flex flex-col gap-3"
      >
        <div className="flex items-center justify-between">
          <h2
            id="usage-summary-entitlements"
            className="text-sm font-semibold text-main"
          >
            {t("usage-summary.entitlements")}
          </h2>
          <Badge variant="secondary">
            {String(subscriptionState ?? "unavailable")}
          </Badge>
        </div>
        {summary.entitlements.length === 0 ? (
          <p className="rounded-lg border border-control-border p-4 text-sm text-control-light">
            {t("usage-summary.no-entitlements")}
          </p>
        ) : (
          <ul className="divide-y divide-control-border rounded-lg border border-control-border">
            {summary.entitlements.map((item) => (
              <li
                key={item.name}
                className="flex items-center justify-between gap-4 p-4 text-sm"
              >
                <span className="min-w-0 truncate text-main">
                  {item.feature}
                </span>
                <span className="shrink-0 text-control-light">
                  {item.enabled
                    ? `${item.limit || "∞"} ${item.unit}`
                    : t("usage-summary.disabled")}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
      <section
        aria-labelledby="usage-summary-aggregates"
        className="flex flex-col gap-3"
      >
        <h2
          id="usage-summary-aggregates"
          className="text-sm font-semibold text-main"
        >
          {t("usage-summary.usage")}
        </h2>
        {summary.aggregates.length === 0 ? (
          <p className="rounded-lg border border-control-border p-4 text-sm text-control-light">
            {t("usage-summary.no-usage")}
          </p>
        ) : (
          <ul className="divide-y divide-control-border rounded-lg border border-control-border">
            {summary.aggregates.map((item) => (
              <li
                key={item.name}
                className="flex items-center justify-between gap-4 p-4 text-sm"
              >
                <span className="text-main">{item.feature}</span>
                <span className="text-control-light">
                  {item.quantity} {item.unit}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
