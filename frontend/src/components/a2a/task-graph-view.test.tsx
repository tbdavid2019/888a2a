import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { type TaskGraphNode, TaskGraphView } from "./task-graph-view";

const nodes: TaskGraphNode[] = [
  {
    id: "root",
    requester: "user-1",
    delegate: "agent-1",
    status: "WORKING",
    artifacts: ["report.md"],
    approvals: ["approved"],
    budget: "10 tokens",
    children: [
      {
        id: "child",
        requester: "agent-1",
        delegate: "agent-2",
        status: "COMPLETED",
        artifacts: [],
        approvals: [],
      },
    ],
  },
];

describe("TaskGraphView", () => {
  it("renders delegation, status, artifacts, approvals, and budget", () => {
    render(<TaskGraphView nodes={nodes} />);
    expect(screen.getByText("root")).toBeInTheDocument();
    expect(screen.getByText(/report\.md/)).toBeInTheDocument();
    expect(screen.getByText(/approved/)).toBeInTheDocument();
    expect(screen.getByText(/10 tokens/)).toBeInTheDocument();
  });

  it("collapses descendants without removing the root", () => {
    render(<TaskGraphView nodes={nodes} />);
    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByText("root")).toBeInTheDocument();
    expect(screen.queryByText("child")).not.toBeInTheDocument();
  });
});
