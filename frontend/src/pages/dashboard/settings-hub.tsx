import { RefreshCw, ShieldAlert, UserX } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { SettingsPage } from "@/components/settings-page";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type HubStatus = {
  hubId: string;
  mode: "closed" | "open" | "public";
  registrationEnabled: boolean;
  peerLeaseSeconds: number;
  maxRegisteredAgents: number;
  maxTasksPerMinute: number;
  maxConcurrentTasks: number;
};

type HubPeer = {
  agentId: string;
  displayName: string;
  providerFamily: string;
  state: string;
};

async function hubRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/hub/v1/${path}`, {
    ...init,
    credentials: "include",
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  const body = (await response.json()) as T & { error?: { message?: string } };
  if (!response.ok) {
    throw new Error(
      body.error?.message ?? `Hub request failed (${response.status})`
    );
  }
  return body;
}

export function SettingsHubPage() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<HubStatus | null>(null);
  const [peers, setPeers] = useState<HubPeer[]>([]);
  const [operatorToken, setOperatorToken] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(
    async (token: string) => {
      setLoading(true);
      setError("");
      try {
        const hubStatus = await hubRequest<HubStatus>("status");
        setStatus(hubStatus);
        const list = await hubRequest<{ agents: HubPeer[] }>("agents", {
          headers: token ? { Authorization: `Bearer ${token}` } : undefined,
        });
        setPeers(list.agents ?? []);
      } catch (cause) {
        setPeers([]);
        setError(
          cause instanceof Error ? cause.message : t("settings.hub.load-failed")
        );
      } finally {
        setLoading(false);
      }
    },
    [t]
  );

  useEffect(() => {
    void load("");
  }, [load]);

  async function setRegistration(enabled: boolean) {
    setBusy(true);
    setError("");
    try {
      await hubRequest("admin/registration", {
        method: "POST",
        headers: { Authorization: `Bearer ${operatorToken}` },
        body: JSON.stringify({ enabled }),
      });
      await load(operatorToken);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("settings.hub.action-failed")
      );
    } finally {
      setBusy(false);
    }
  }

  async function revoke(agentId: string) {
    setBusy(true);
    setError("");
    try {
      await hubRequest(`admin/agents/${encodeURIComponent(agentId)}/revoke`, {
        method: "POST",
        headers: { Authorization: `Bearer ${operatorToken}` },
        body: JSON.stringify({ reason: "Revoked by Hub operator" }),
      });
      await load(operatorToken);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("settings.hub.action-failed")
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <SettingsPage
      title={t("settings.hub.title")}
      description={t("settings.hub.description")}
      actions={
        <Button
          variant="outline"
          onClick={() => void load(operatorToken)}
          disabled={loading}
        >
          <RefreshCw className="size-4" />
          {t("settings.hub.refresh")}
        </Button>
      }
    >
      {error && <Alert variant="error">{error}</Alert>}
      {loading && !status ? (
        <p className="text-sm text-control-light">{t("common.loading")}</p>
      ) : status ? (
        <div className="flex flex-col gap-4">
          <div className="rounded-lg border border-border bg-card p-4 flex flex-col gap-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h2 className="font-medium text-main">{status.hubId}</h2>
                <p className="text-sm text-control-light">
                  {t("settings.hub.mode")}
                </p>
              </div>
              <Badge
                variant={status.mode === "public" ? "destructive" : "secondary"}
              >
                {status.mode}
              </Badge>
            </div>
            <p className="text-sm text-control-light">
              {status.mode === "public"
                ? t("settings.hub.public-warning")
                : t("settings.hub.private-note")}
            </p>
            <div className="grid grid-cols-2 gap-2 text-sm text-control-light sm:grid-cols-4">
              <span>
                {t("settings.hub.agent-limit", {
                  n: status.maxRegisteredAgents,
                })}
              </span>
              <span>
                {t("settings.hub.task-limit", { n: status.maxTasksPerMinute })}
              </span>
              <span>
                {t("settings.hub.concurrent-limit", {
                  n: status.maxConcurrentTasks,
                })}
              </span>
              <span>
                {t("settings.hub.lease", { n: status.peerLeaseSeconds })}
              </span>
            </div>
          </div>
          <div className="rounded-lg border border-border bg-card p-4 flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <ShieldAlert className="size-4 text-control-light" />
              <h2 className="font-medium text-main">
                {t("settings.hub.operator-title")}
              </h2>
            </div>
            <p className="text-sm text-control-light">
              {t("settings.hub.operator-note")}
            </p>
            <Input
              type="password"
              value={operatorToken}
              onChange={(event) => setOperatorToken(event.target.value)}
              placeholder={t("settings.hub.operator-placeholder")}
              autoComplete="off"
            />
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={!operatorToken || busy}
                onClick={() => void load(operatorToken)}
              >
                {t("settings.hub.load-peers")}
              </Button>
              <Button
                variant="outline"
                disabled={!operatorToken || busy}
                onClick={() =>
                  void setRegistration(!status.registrationEnabled)
                }
              >
                {status.registrationEnabled
                  ? t("settings.hub.disable-registration")
                  : t("settings.hub.enable-registration")}
              </Button>
            </div>
          </div>
          <div className="rounded-lg border border-border bg-card p-4 flex flex-col gap-3">
            <h2 className="font-medium text-main">
              {t("settings.hub.peers-title")}
            </h2>
            {peers.length === 0 ? (
              <p className="text-sm text-control-light">
                {t("settings.hub.no-peers")}
              </p>
            ) : (
              peers.map((peer) => (
                <div
                  key={peer.agentId}
                  className="flex items-center justify-between gap-3 border-t border-border pt-3"
                >
                  <div className="min-w-0">
                    <p className="truncate font-medium text-main">
                      {peer.displayName}
                    </p>
                    <p className="truncate text-xs text-control-light">
                      {peer.agentId} · {peer.providerFamily} · {peer.state}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!operatorToken || busy}
                    onClick={() => void revoke(peer.agentId)}
                    aria-label={t("settings.hub.revoke")}
                  >
                    <UserX className="size-4" />
                  </Button>
                </div>
              ))
            )}
          </div>
        </div>
      ) : null}
    </SettingsPage>
  );
}
