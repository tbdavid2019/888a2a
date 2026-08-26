import { useEffect } from "react";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAppStore } from "@/stores";
import { OrganizationState } from "@/types/proto-es/a2a888/organization_pb";

export interface OrgSwitcherProps {
  className?: string;
}

export function OrgSwitcher({ className }: OrgSwitcherProps) {
  const currentOrgId = useAppStore((s) => s.currentOrganizationId);
  const organizations = useAppStore((s) => s.organizations);
  const fetchOrganizations = useAppStore((s) => s.fetchOrganizations);
  const switchOrganization = useAppStore((s) => s.switchOrganization);

  useEffect(() => {
    fetchOrganizations();
  }, [fetchOrganizations]);

  const currentOrg = organizations.find((o) => o.id === currentOrgId);
  const isSuspended =
    currentOrg?.state === OrganizationState.SUSPENDED ||
    currentOrg?.state === OrganizationState.CLOSED;

  return (
    <div
      className={`flex items-center gap-2 ${className ?? ""}`}
      data-testid="org-switcher"
    >
      <Select
        value={currentOrgId}
        onValueChange={(val) => {
          if (val && val !== currentOrgId) {
            switchOrganization(val);
          }
        }}
      >
        <SelectTrigger
          className="w-[180px] h-8 text-xs font-medium"
          data-testid="org-switcher-trigger"
        >
          <SelectValue placeholder="Select Organization">
            {currentOrg ? currentOrg.name : "Default Organization"}
          </SelectValue>
        </SelectTrigger>
        <SelectContent data-testid="org-switcher-content">
          {organizations.map((org) => (
            <SelectItem
              key={org.id}
              value={org.id}
              data-testid={`org-option-${org.id}`}
            >
              <div className="flex items-center justify-between w-full gap-2">
                <span>{org.name}</span>
                {org.state === OrganizationState.SUSPENDED && (
                  <Badge
                    variant="destructive"
                    className="text-[10px] px-1 py-0"
                  >
                    Suspended
                  </Badge>
                )}
                {org.state === OrganizationState.CLOSED && (
                  <Badge
                    variant="destructive"
                    className="text-[10px] px-1 py-0"
                  >
                    Closed
                  </Badge>
                )}
              </div>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {isSuspended && (
        <Badge
          variant="destructive"
          className="text-xs"
          data-testid="org-suspended-badge"
        >
          Tenant Suspended (Read-Only)
        </Badge>
      )}
    </div>
  );
}
