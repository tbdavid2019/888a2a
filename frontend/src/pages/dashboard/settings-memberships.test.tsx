import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAppStore } from "@/stores";
import {
  MembershipState,
  OrganizationMembershipSchema,
  OrganizationRole,
} from "@/types/proto-es/a2a888/organization_pb";
import { SettingsMembershipsPage } from "./settings-memberships";

const mock = vi.hoisted(() => ({
  listMemberships: vi.fn(),
  addMembership: vi.fn(),
  updateMembership: vi.fn(),
  removeMembership: vi.fn(),
}));

vi.mock("@/connect", () => ({
  organizationServiceClient: {
    listMemberships: mock.listMemberships,
    addMembership: mock.addMembership,
    updateMembership: mock.updateMembership,
    removeMembership: mock.removeMembership,
  },
}));

function membership(principalId: string) {
  return create(OrganizationMembershipSchema, {
    organizationId: "org-a",
    principalId,
    role: OrganizationRole.MEMBER,
    state: MembershipState.ACTIVE,
  });
}

beforeEach(() => {
  useAppStore.setState({ currentOrganizationId: "org-a" } as never);
  mock.listMemberships.mockReset();
  mock.addMembership.mockReset();
  mock.updateMembership.mockReset();
  mock.removeMembership.mockReset();
  mock.listMemberships.mockResolvedValue({ memberships: [membership("101")] });
  mock.addMembership.mockResolvedValue(membership("202"));
  mock.updateMembership.mockResolvedValue(membership("101"));
  mock.removeMembership.mockResolvedValue({});
});

describe("settings-memberships", () => {
  it("lists memberships and adds a principal to the active organization", async () => {
    render(<SettingsMembershipsPage />);
    expect(await screen.findByTestId("membership-101")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("101"), {
      target: { value: "202" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add member" }));

    await waitFor(() => {
      expect(mock.addMembership).toHaveBeenCalledWith(
        expect.objectContaining({
          membership: expect.objectContaining({
            organizationId: "org-a",
            principalId: "202",
          }),
        })
      );
    });
  });

  it("renders every membership independently for multi-membership administration", async () => {
    mock.listMemberships.mockResolvedValue({
      memberships: [membership("101"), membership("202")],
    });
    render(<SettingsMembershipsPage />);
    expect(await screen.findByTestId("membership-101")).toBeInTheDocument();
    expect(screen.getByTestId("membership-202")).toBeInTheDocument();
  });

  it("disables membership mutations for a suspended organization", async () => {
    useAppStore.setState({
      organizations: [
        {
          id: "org-a",
          state: 2,
          name: "Suspended",
        },
      ],
    } as never);
    render(<SettingsMembershipsPage />);
    expect(
      await screen.findByTestId("membership-read-only")
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add member" })).toBeDisabled();
    expect(mock.addMembership).not.toHaveBeenCalled();
  });

  it("hides membership data when the server denies organization administration", async () => {
    mock.listMemberships.mockRejectedValue(
      new ConnectError("denied", Code.PermissionDenied)
    );
    render(<SettingsMembershipsPage />);
    expect(
      await screen.findByText(
        "You do not have permission to administer organization memberships."
      )
    ).toBeInTheDocument();
    expect(screen.queryByTestId("membership-101")).toBeNull();
  });
});
