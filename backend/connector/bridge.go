package connector

import "errors"

type BridgePolicy struct {
	OrganizationID string
	Source         string
	Destinations   map[string]bool
}

func (p BridgePolicy) Allows(organizationID, source, destination string) bool {
	return p.OrganizationID != "" && p.OrganizationID == organizationID && p.Source == source && p.Destinations != nil && p.Destinations[destination]
}

type Divergence struct {
	OrganizationID string
	InstallationID string
	Source         string
	Destination    string
	EventID        string
	Reason         string
}

func (d Divergence) Validate() error {
	if d.OrganizationID == "" || d.InstallationID == "" || d.Source == "" || d.Destination == "" || d.EventID == "" || d.Reason == "" {
		return errors.New("connector divergence requires tenant, installation, source, destination, event, and reason")
	}
	return nil
}
