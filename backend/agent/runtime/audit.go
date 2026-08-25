package agentruntime

import (
	"regexp"
	"strings"
	"time"
)

var (
	// sensitivePattern matches secret key names or bearer tokens.
	sensitiveKeyPattern = regexp.MustCompile(`(?i)(token|password|secret|auth|bearer|apikey|api_key|credential|private_key)`)
)

// AuditEvent records runtime preparation, verification, quarantine, and execution actions.
type AuditEvent struct {
	Timestamp      time.Time `json:"timestamp"`
	Action         string    `json:"action"` // e.g. PREPARE, VERIFY, QUARANTINE, REMOVE, RETRY
	ProviderID     string    `json:"providerId"`
	IdentityDigest string    `json:"identityDigest"`
	Success        bool      `json:"success"`
	Reason         string    `json:"reason,omitempty"`
	Details        string    `json:"details,omitempty"`
}

// SanitizeAuditString removes potentially sensitive values such as tokens or secrets.
func SanitizeAuditString(input string) string {
	if input == "" {
		return ""
	}
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		if sensitiveKeyPattern.MatchString(line) {
			lines[i] = "[REDACTED]"
		}
	}
	return strings.Join(lines, "\n")
}

// NewAuditEvent creates a sanitized AuditEvent.
func NewAuditEvent(action, providerID, digest string, success bool, reason, details string) AuditEvent {
	return AuditEvent{
		Timestamp:      time.Now().UTC(),
		Action:         action,
		ProviderID:     providerID,
		IdentityDigest: digest,
		Success:        success,
		Reason:         SanitizeAuditString(reason),
		Details:        SanitizeAuditString(details),
	}
}
