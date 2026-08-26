import { create } from "@bufbuild/protobuf";
import { organizationServiceClient } from "@/connect";
import type {
  Organization,
  OrganizationMembership,
  Workspace,
} from "@/types/proto-es/a2a888/organization_pb";
import {
  ListOrganizationsRequestSchema,
  ListWorkspacesRequestSchema,
  SwitchOrganizationRequestSchema,
} from "@/types/proto-es/a2a888/organization_pb";
import type { AppSliceCreator, OrganizationSlice } from "./types";

export const createOrganizationSlice: AppSliceCreator<OrganizationSlice> = (
  set,
  get
) => ({
  currentOrganizationId: "default",
  organizations: [],
  workspaces: [],
  memberships: [],

  setCurrentOrganizationId: (orgId: string) => {
    set({ currentOrganizationId: orgId });
  },

  setOrganizations: (orgs: Organization[]) => {
    set({ organizations: orgs });
  },

  setWorkspaces: (workspaces: Workspace[]) => {
    set({ workspaces });
  },

  setMemberships: (memberships: OrganizationMembership[]) => {
    set({ memberships });
  },

  fetchOrganizations: async () => {
    try {
      const res = await organizationServiceClient.listOrganizations(
        create(ListOrganizationsRequestSchema, {})
      );
      const organizationId = res.activeOrganizationId || "default";
      const workspaceResponse = await organizationServiceClient.listWorkspaces(
        create(ListWorkspacesRequestSchema, { organizationId })
      );
      set({
        organizations: res.organizations,
        currentOrganizationId: organizationId,
        workspaces: workspaceResponse.workspaces,
      });
    } catch {
      // Graceful fallback for non-auth or single-tenant mode
    }
  },

  fetchWorkspaces: async (organizationId) => {
    const id = organizationId || "default";
    try {
      const res = await organizationServiceClient.listWorkspaces(
        create(ListWorkspacesRequestSchema, { organizationId: id })
      );
      set({ workspaces: res.workspaces });
    } catch {
      set({ workspaces: [] });
    }
  },

  switchOrganization: async (orgId: string) => {
    const res = await organizationServiceClient.switchOrganization(
      create(SwitchOrganizationRequestSchema, { organizationId: orgId })
    );
    if (res.organization) {
      try {
        localStorage.setItem("888a2a-active-organization", orgId);
      } catch {
        // Storage may be disabled; the server-side selection remains authoritative.
      }
      set({
        currentOrganizationId: orgId,
        channels: [],
        agents: [],
        chatMessages: {},
        threadByRoot: {},
        workspaces: [],
        memberships: [],
      });
      await get().fetchWorkspaces(orgId);
    }
  },
});
