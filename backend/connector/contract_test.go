package connector

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type fixtureConnector struct{}

func (fixtureConnector) Manifest() Manifest {
	return Manifest{Kind: "fixture", Contract: ContractVersion, Capabilities: CapabilityMatrix{
		CapabilityVerify: true, CapabilityNormalize: true, CapabilityText: true,
	}}
}
func (f fixtureConnector) VerifyInbound(_ context.Context, i Installation, h http.Header, raw []byte) (VerifiedInbound, error) {
	return VerifiedInbound{Installation: i, ExternalID: h.Get("X-Event-ID"), ReceivedAt: time.Now(), Headers: h, Raw: raw}, nil
}
func (fixtureConnector) Normalize(_ context.Context, in VerifiedInbound) (Envelope, error) {
	return Envelope{OrganizationID: in.Installation.OrganizationID, InstallationID: in.Installation.InstallationID, ExternalEventID: in.ExternalID, EventType: "message"}, nil
}
func (fixtureConnector) Deliver(context.Context, Installation, Outbound) (DeliveryResult, error) {
	return DeliveryResult{ExternalID: "fixture-delivery"}, nil
}

func TestConnectorContractAndTenantIdentity(t *testing.T) {
	var adapter Connector = fixtureConnector{}
	if err := adapter.Manifest().Validate(); err != nil {
		t.Fatal(err)
	}
	installation := Installation{OrganizationID: "org-a", InstallationID: "line-a", Kind: "fixture"}
	if err := installation.Validate(); err != nil {
		t.Fatal(err)
	}
	inbound, err := adapter.VerifyInbound(context.Background(), installation, http.Header{"X-Event-Id": []string{"event-1"}}, []byte(`{"text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := adapter.Normalize(context.Background(), inbound)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.OrganizationID != "org-a" || envelope.InstallationID != "line-a" || envelope.ExternalEventID != "event-1" {
		t.Fatalf("envelope lost tenant identity: %+v", envelope)
	}
}

func TestCapabilityMatrixDoesNotSimulateUnsupportedOperations(t *testing.T) {
	manifest := fixtureConnector{}.Manifest()
	if manifest.Capabilities.Supports(CapabilityEdits) || manifest.Capabilities.Supports(CapabilityRecalls) {
		t.Fatal("fixture must not claim unsupported operations")
	}
}

func TestReceiveAuthenticatesBeforeWritingInbox(t *testing.T) {
	installation := Installation{OrganizationID: "org-a", InstallationID: "install-a", Kind: "fixture"}
	writes := 0
	envelope, accepted, err := Receive(context.Background(), fixtureConnector{}, installation, http.Header{"X-Event-Id": []string{"event-2"}}, []byte(`{"text":"hello"}`), func(_ context.Context, record InboxRecord) (bool, error) {
		writes++
		if record.OrganizationID != installation.OrganizationID {
			t.Fatal("inbox record lost tenant")
		}
		return true, nil
	})
	if err != nil || !accepted || envelope.ExternalEventID != "event-2" || writes != 1 {
		t.Fatalf("receive = %+v, %v, writes=%d", envelope, err, writes)
	}
}
