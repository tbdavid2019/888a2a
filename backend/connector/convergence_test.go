package connector

import (
	"testing"
	"time"
)

func TestConvergeDeduplicatesAndOrdersLINESlackAndWidgetFixtures(t *testing.T) {
	arrived := []Envelope{
		{OrganizationID: "org-a", InstallationID: "widget-a", ExternalEventID: "widget-2", OccurredAt: time.Unix(20, 0)},
		{OrganizationID: "org-a", InstallationID: "line-a", ExternalEventID: "line-1", OccurredAt: time.Unix(10, 0)},
		{OrganizationID: "org-a", InstallationID: "slack-a", ExternalEventID: "slack-1", OccurredAt: time.Unix(15, 0)},
		{OrganizationID: "org-a", InstallationID: "line-a", ExternalEventID: "line-1", OccurredAt: time.Unix(10, 0)},
		{OrganizationID: "org-a", InstallationID: "widget-a", ExternalEventID: "widget-1", OccurredAt: time.Unix(5, 0)},
	}
	first := Converge(arrived)
	second := Converge([]Envelope{arrived[4], arrived[2], arrived[0], arrived[3], arrived[1]})
	if len(first) != 4 || len(second) != 4 {
		t.Fatalf("dedup lengths = %d, %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ExternalEventID != second[i].ExternalEventID {
			t.Fatalf("convergence differs at %d: %q != %q", i, first[i].ExternalEventID, second[i].ExternalEventID)
		}
	}
}
