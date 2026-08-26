package tools

import (
	"context"

	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// WorkStore defines the persistence interface required by A2A agent tools.
// Both the manager's PostgreSQL store (*store.Store) and in-memory test stores implement this interface.
type WorkStore interface {
	EnsureWorkContext(ctx context.Context, tenantID, contextID, rootWorkID string) (*store.WorkContextMessage, error)
	CreateWork(ctx context.Context, work *store.WorkMessage) error
	GetWork(ctx context.Context, tenantID, workID string) (*store.WorkMessage, error)
	GetWorkByA2ATaskID(ctx context.Context, tenantID, a2aTaskID string) (*store.WorkMessage, error)
	GetWorkByIdempotencyKey(ctx context.Context, tenantID, requesterAgentID, idempotencyKey string) (*store.WorkMessage, error)
	UpdateWorkState(ctx context.Context, tenantID, workID string, expectedVersion uint64, newState string, terminalReason string) (uint64, error)
	ListWork(ctx context.Context, filter store.ListWorkFilter) ([]*store.WorkMessage, int, error)
	CreateWorkArtifact(ctx context.Context, artifact *store.WorkArtifactMessage) error
	ListWorkArtifacts(ctx context.Context, tenantID, workID string) ([]*store.WorkArtifactMessage, error)
	AppendWorkEvent(ctx context.Context, event *store.WorkEventMessage) error
	GetLatestWorkEventSequence(ctx context.Context, tenantID, workID string) (uint64, error)
}
