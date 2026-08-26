import { useAppStore } from "./index";

// Hooks that read the session's permission set live in their own module (not
// in auth.ts) to break the index <-> auth circular import: index.ts composes
// every slice including auth's, so auth importing the store at module scope
// would put createAuthSlice in a temporal dead zone whenever auth.ts happened
// to be evaluated first. Components import this module instead.

// useHasPermission reports whether the current caller holds a workspace-scope
// permission, sourced from the server-populated User.permissions set
// (GetCurrentUser). Per-resource permissions such as laelia.agents.edit are not
// represented here — agents.edit is surfaced per-agent as Agent.canEdit, since
// the creator (agentEditor binding) and workspace admins resolve it per
// resource. Subscribe via the hook so UI re-renders when the session loads.
function hasPermissionValue(
  permissions: string[] | undefined,
  permission: string
): boolean {
  if (!permissions) return false;
  if (permissions.includes(permission)) return true;
  if (permission.startsWith("888a2a.")) {
    return permissions.includes(`laelia.${permission.slice("888a2a.".length)}`);
  }
  if (permission.startsWith("laelia.")) {
    return permissions.includes(`888a2a.${permission.slice("laelia.".length)}`);
  }
  return false;
}

export function useHasPermission(perm: string): boolean {
  return useAppStore((s) =>
    hasPermissionValue(s.currentUser?.permissions, perm)
  );
}

// hasPermission is the non-reactive variant for use inside callbacks/effects
// where subscribing to the store is undesirable.
export function hasPermission(perm: string): boolean {
  return hasPermissionValue(
    useAppStore.getState().currentUser?.permissions,
    perm
  );
}
