import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApprovalCenterPage } from "./approval-center";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("@/stores/permissions", () => ({
  useHasPermission: () => true,
}));

describe("ApprovalCenterPage", () => {
  it("renders the organization approval center shell", () => {
    render(<ApprovalCenterPage requests={[]} onDecision={vi.fn()} />);

    expect(screen.getByText("approval-center.title")).toBeInTheDocument();
    expect(screen.getByText("approval-center.description")).toBeInTheDocument();
  });
});
