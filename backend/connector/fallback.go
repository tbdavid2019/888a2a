package connector

import "fmt"

// CapabilityFallback converts an unsupported operation into an explicit
// divergence result. It must not claim delivery succeeded.
func CapabilityFallback(manifest Manifest, capability Capability, source, destination, eventID string) (Divergence, error) {
	if err := manifest.Validate(); err != nil {
		return Divergence{}, err
	}
	if manifest.Capabilities.Supports(capability) {
		return Divergence{}, nil
	}
	return Divergence{Source: source, Destination: destination, EventID: eventID, Reason: fmt.Sprintf("connector %s does not support %s", manifest.Kind, capability)}, nil
}
