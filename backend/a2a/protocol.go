// Package a2a owns the boundary between 888a2a and the official A2A SDK.
//
// Keep SDK types inside this package. Other packages should depend on the
// small protocol report and on 888a2a-owned contracts added here over time.
package a2a

import (
	"strings"

	officiala2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/pkg/errors"
)

const (
	// Source: https://github.com/a2aproject/a2a-go/blob/v2.4.0/a2a/core.go
	officialSDKModulePath = "github.com/a2aproject/a2a-go/v2"
	officialSDKVersion    = "v2.4.0"

	// Canonical A2A 1.0 version constant.
	ProtocolVersion1_0 = "1.0"
)

// ErrUnsupportedProtocolVersion indicates an incompatible protocol version was requested.
var ErrUnsupportedProtocolVersion = errors.New("unsupported A2A protocol version")

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

// NegotiateProtocolVersion validates the client's requested A2A version.
// If empty, it defaults to 1.0. If compatible (major version 1), it returns "1.0".
// Otherwise, it returns ErrUnsupportedProtocolVersion.
func NegotiateProtocolVersion(reqVersion string) (string, error) {
	v := strings.TrimSpace(reqVersion)
	if v == "" || v == ProtocolVersion1_0 || v == string(officiala2a.Version) {
		return ProtocolVersion1_0, nil
	}

	parts := strings.Split(v, ".")
	if len(parts) > 0 && parts[0] == "1" {
		return ProtocolVersion1_0, nil
	}

	return "", errors.Wrapf(ErrUnsupportedProtocolVersion, "requested %q, supported %q", reqVersion, ProtocolVersion1_0)
}

// FormatVersionHeader returns the standard A2A-Version header value.
func FormatVersionHeader() string {
	return string(officiala2a.Version)
}
