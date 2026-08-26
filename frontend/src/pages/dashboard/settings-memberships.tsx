import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { useCallback, useEffect, useState } from "react";
import {
  PageLoading,
  PermissionNotice,
  SettingsPage,
} from "@/components/settings-page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { organizationServiceClient } from "@/connect";
import { useAppStore } from "@/stores";
import {
  AddMembershipRequestSchema,
  MembershipState,
  type OrganizationMembership,
  OrganizationMembershipSchema,
  OrganizationRole,
  OrganizationState,
  RemoveMembershipRequestSchema,
  UpdateMembershipRequestSchema,
} from "@/types/proto-es/a2a888/organization_pb";

const roleOptions = [
  [OrganizationRole.OWNER, "Owner"],
  [OrganizationRole.ADMIN, "Admin"],
  [OrganizationRole.MEMBER, "Member"],
  [OrganizationRole.GUEST, "Guest"],
  [OrganizationRole.BILLING_ADMIN, "Billing admin"],
  [OrganizationRole.AGENT_ADMIN, "Agent admin"],
  [OrganizationRole.APPROVER, "Approver"],
] as const;

const stateOptions = [
  [MembershipState.ACTIVE, "Active"],
  [MembershipState.SUSPENDED, "Suspended"],
  [MembershipState.INVITED, "Invited"],
] as const;

function roleLabel(role: OrganizationRole): string {
  return roleOptions.find(([value]) => value === role)?.[1] ?? "Member";
}

export function SettingsMembershipsPage() {
  const organizationID = useAppStore((state) => state.currentOrganizationId);
  const organization = useAppStore((state) =>
    state.organizations.find((candidate) => candidate.id === organizationID)
  );
  const [memberships, setMemberships] = useState<OrganizationMembership[]>([]);
  const [principalID, setPrincipalID] = useState("");
  const [role, setRole] = useState(OrganizationRole.MEMBER);
  const [state, setState] = useState(MembershipState.ACTIVE);
  const [loading, setLoading] = useState(true);
  const [denied, setDenied] = useState(false);
  const [error, setError] = useState("");
  const isReadOnly =
    organization?.state === OrganizationState.SUSPENDED ||
    organization?.state === OrganizationState.CLOSED;

  const load = useCallback(async () => {
    if (!organizationID) return;
    setLoading(true);
    setDenied(false);
    try {
      const response = await organizationServiceClient.listMemberships({
        organizationId: organizationID,
      });
      setMemberships(response.memberships);
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.PermissionDenied) {
        setDenied(true);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setLoading(false);
    }
  }, [organizationID]);

  useEffect(() => {
    load();
  }, [load]);

  async function addMembership() {
    if (isReadOnly || !organizationID || !principalID.trim()) return;
    setError("");
    try {
      await organizationServiceClient.addMembership(
        create(AddMembershipRequestSchema, {
          membership: create(OrganizationMembershipSchema, {
            organizationId: organizationID,
            principalId: principalID.trim(),
            role,
            state,
          }),
        })
      );
      setPrincipalID("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function updateMembership(
    membership: OrganizationMembership,
    nextRole: OrganizationRole
  ) {
    if (isReadOnly) return;
    setError("");
    try {
      await organizationServiceClient.updateMembership(
        create(UpdateMembershipRequestSchema, {
          membership: create(OrganizationMembershipSchema, {
            ...membership,
            role: nextRole,
          }),
        })
      );
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function removeMembership(membership: OrganizationMembership) {
    if (isReadOnly) return;
    setError("");
    try {
      await organizationServiceClient.removeMembership(
        create(RemoveMembershipRequestSchema, {
          organizationId: organizationID,
          principalId: membership.principalId,
        })
      );
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  if (denied) {
    return (
      <PermissionNotice message="You do not have permission to administer organization memberships." />
    );
  }
  if (loading) return <PageLoading />;

  return (
    <SettingsPage
      title="Organization members"
      description="Manage tenant memberships, roles, and lifecycle state."
    >
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {isReadOnly && (
        <p
          role="status"
          className="text-sm text-control-light"
          data-testid="membership-read-only"
        >
          This organization is suspended or closed. Membership changes are
          disabled.
        </p>
      )}
      <div
        className="flex flex-wrap items-end gap-2"
        data-testid="membership-add-form"
      >
        <label className="flex flex-col gap-1 text-xs text-control-light">
          Principal ID
          <Input
            disabled={isReadOnly}
            value={principalID}
            onChange={(event) => setPrincipalID(event.target.value)}
            placeholder="101"
          />
        </label>
        <label className="flex flex-col gap-1 text-xs text-control-light">
          Role
          <select
            disabled={isReadOnly}
            aria-label="Membership role"
            value={role}
            onChange={(event) =>
              setRole(Number(event.target.value) as OrganizationRole)
            }
            className="h-9 rounded-md border bg-background px-2 text-sm"
          >
            {roleOptions.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-control-light">
          State
          <select
            disabled={isReadOnly}
            aria-label="Membership state"
            value={state}
            onChange={(event) =>
              setState(Number(event.target.value) as MembershipState)
            }
            className="h-9 rounded-md border bg-background px-2 text-sm"
          >
            {stateOptions.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <Button
          type="button"
          onClick={addMembership}
          disabled={isReadOnly || !principalID.trim()}
        >
          Add member
        </Button>
      </div>
      {memberships.length === 0 ? (
        <p className="text-sm text-control-light">No organization members.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Principal</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>State</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {memberships.map((membership) => (
              <TableRow
                key={membership.principalId}
                data-testid={`membership-${membership.principalId}`}
              >
                <TableCell>{membership.principalId}</TableCell>
                <TableCell>
                  <select
                    disabled={isReadOnly}
                    aria-label={`Role for ${membership.principalId}`}
                    value={membership.role}
                    onChange={(event) =>
                      updateMembership(
                        membership,
                        Number(event.target.value) as OrganizationRole
                      )
                    }
                    className="rounded border bg-background px-2 py-1 text-sm"
                  >
                    {roleOptions.map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                  </select>
                </TableCell>
                <TableCell>
                  <Badge
                    variant={
                      membership.state === MembershipState.ACTIVE
                        ? "secondary"
                        : "destructive"
                    }
                  >
                    {membership.state === MembershipState.ACTIVE
                      ? roleLabel(membership.role)
                      : stateOptions.find(
                          ([value]) => value === membership.state
                        )?.[1]}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Button
                    type="button"
                    variant="ghost"
                    disabled={isReadOnly}
                    onClick={() => removeMembership(membership)}
                  >
                    Remove
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </SettingsPage>
  );
}
