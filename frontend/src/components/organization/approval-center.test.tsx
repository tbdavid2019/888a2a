import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApprovalCenter, type ApprovalCenterRequest } from "./approval-center";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

const request: ApprovalCenterRequest = {
  name: "approvalRequests/req-1",
  requester: "alice@example.com",
  agent: "agents/finance",
  actionType: "payments.create",
  resource: "invoices/42",
  destination: "stripe",
  risk: "high",
  parameters: {
    amount: 250,
    currency: "USD",
    apiToken: "should-not-render",
  },
  approvalCount: 1,
  requiredApprovals: 2,
  expiresAt: "2026-08-27T12:00:00.000Z",
  eligible: true,
  state: "pending",
};

describe("ApprovalCenter", () => {
  it("shows bounded intent only after an eligible approver selects a request", () => {
    render(
      <ApprovalCenter requests={[request]} onDecision={vi.fn()} canView />
    );

    expect(screen.getByText("invoices/42")).toBeInTheDocument();
    expect(screen.queryByText("should-not-render")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /invoices\/42/i }));

    expect(screen.getByText("payments.create")).toBeInTheDocument();
    expect(screen.getByText("250")).toBeInTheDocument();
    expect(
      screen.getByText("approval-center.intent-hash-hidden")
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "approval-center.approve" })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "approval-center.deny" })
    ).toBeInTheDocument();
  });

  it("does not render sensitive requests to an ineligible user", () => {
    render(
      <ApprovalCenter
        requests={[{ ...request, eligible: false }]}
        onDecision={vi.fn()}
        canView
      />
    );

    expect(
      screen.getByText("approval-center.no-eligible-requests")
    ).toBeInTheDocument();
    expect(screen.queryByText("invoices/42")).not.toBeInTheDocument();
    expect(screen.queryByText("alice@example.com")).not.toBeInTheDocument();
  });

  it("does not expose any requests without the organization approval permission", () => {
    render(
      <ApprovalCenter
        requests={[request]}
        onDecision={vi.fn()}
        canView={false}
      />
    );

    expect(screen.getByText("approval-center.not-allowed")).toBeInTheDocument();
    expect(screen.queryByText("invoices/42")).not.toBeInTheDocument();
  });

  it("submits an approval decision for the selected request", () => {
    const onDecision = vi.fn();
    render(
      <ApprovalCenter requests={[request]} onDecision={onDecision} canView />
    );

    fireEvent.click(screen.getByRole("button", { name: /invoices\/42/i }));
    fireEvent.click(
      screen.getByRole("button", { name: "approval-center.approve" })
    );

    expect(onDecision).toHaveBeenCalledWith(request.name, "approve");
  });
});
