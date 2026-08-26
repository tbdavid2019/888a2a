import type { AppSliceCreator, OrganizationSlice } from "./types";
import type {
  Organization,
  OrganizationMembership,
  Workspace,
} from "@/types/proto-es/a2a888/organization_pb";

export const createOrganizationSlice: AppSliceCreator<OrganizationSlice> = (
  set
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
});
