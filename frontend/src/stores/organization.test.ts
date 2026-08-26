import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
  OrganizationSchema,
  OrganizationState,
  WorkspaceSchema,
} from "@/types/proto-es/a2a888/organization_pb";
import { useAppStore } from "./index";

describe("OrganizationSlice", () => {
  it("initializes with default organization id", () => {
    const state = useAppStore.getState();
    expect(state.currentOrganizationId).toBe("default");
    expect(state.organizations).toEqual([]);
    expect(state.workspaces).toEqual([]);
    expect(state.memberships).toEqual([]);
  });

  it("updates active organization and state", () => {
    const org = create(OrganizationSchema, {
      id: "org-acme",
      name: "Acme Corp",
      slug: "acme",
      state: OrganizationState.ACTIVE,
    });

    const ws = create(WorkspaceSchema, {
      id: "ws-eng",
      organizationId: "org-acme",
      name: "Engineering",
      slug: "eng",
      isDefault: true,
    });

    useAppStore.getState().setOrganizations([org]);
    useAppStore.getState().setWorkspaces([ws]);
    useAppStore.getState().setCurrentOrganizationId("org-acme");

    expect(useAppStore.getState().currentOrganizationId).toBe("org-acme");
    expect(useAppStore.getState().organizations).toHaveLength(1);
    expect(useAppStore.getState().organizations[0].name).toBe("Acme Corp");
    expect(useAppStore.getState().workspaces[0].name).toBe("Engineering");
  });
});
