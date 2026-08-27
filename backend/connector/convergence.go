package connector

import "sort"

// Converge removes duplicate installation events and returns a deterministic
// order. Platform arrival order is intentionally ignored.
func Converge(events []Envelope) []Envelope {
	seen := make(map[string]struct{}, len(events))
	result := make([]Envelope, 0, len(events))
	for _, event := range events {
		key := event.OrganizationID + "\x00" + event.InstallationID + "\x00" + event.ExternalEventID
		if event.ExternalEventID == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, event)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].OccurredAt.Equal(result[j].OccurredAt) {
			return result[i].ExternalEventID < result[j].ExternalEventID
		}
		return result[i].OccurredAt.Before(result[j].OccurredAt)
	})
	return result
}
