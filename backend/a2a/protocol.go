// Package a2a owns the boundary between 888a2a and the official A2A SDK.
//
// Keep SDK types inside this package. Other packages should depend on the
// small protocol report and on 888a2a-owned contracts added here over time.
package a2a

import officiala2a "github.com/a2aproject/a2a-go/v2/a2a"

const (
	// Source: https://github.com/a2aproject/a2a-go/blob/v2.4.0/a2a/core.go
	officialSDKModulePath = "github.com/a2aproject/a2a-go/v2"
	officialSDKVersion    = "v2.4.0"
)

// ProtocolReport describes the A2A protocol supported by this build.
//
// SDK provenance is included so compatibility reports can identify the
// implementation evidence without exposing SDK-specific types.
type ProtocolReport struct {
	Protocol   string `json:"protocol"`
	Version    string `json:"version"`
	SDKModule  string `json:"sdkModule"`
	SDKVersion string `json:"sdkVersion"`
}

// SupportedProtocol returns the protocol and official SDK provenance used by
// the Agent Network boundary.
func SupportedProtocol() ProtocolReport {
	return ProtocolReport{
		Protocol:   "A2A",
		Version:    string(officiala2a.Version),
		SDKModule:  officialSDKModulePath,
		SDKVersion: officialSDKVersion,
	}
}
