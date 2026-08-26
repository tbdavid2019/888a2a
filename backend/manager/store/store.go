package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/tbdavid2019/888a2a/backend/common"
	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
)

type Store struct {
	Secret        string
	dbConnManager *DBConnectionManager
	enableCache   bool

	// webPushSender dispatches Web Push notifications for directed messages. It
	// is injected via SetWebPushSender after construction to avoid a circular
	// dependency: the webpush component imports the store, so the store cannot
	// import it back. Nil (the default) disables push dispatch —
	// generateActivityRows treats a nil sender as a no-op.
	webPushSender WebPushSender

	// roomNotifier wakes long-polling readers when a conversation's room
	// version changes. Injected via SetRoomNotifier after construction; nil
	// (the default) disables wake-ups (long polls run to their timeout).
	roomNotifier RoomNotifier

	userIDCache            *lru.Cache[int, *UserMessage]
	userEmailCache         *lru.Cache[string, *UserMessage]
	userHandleCache        *lru.Cache[string, *UserMessage]
	settingCache           *lru.Cache[models.SettingName, *SettingMessage]
	policyCache            *lru.Cache[string, *PolicyMessage]
	idpCache               *lru.Cache[string, *IdentityProviderMessage]
	groupCache             *lru.Cache[string, *GroupMessage]
	agentIDCache           *lru.Cache[string, *AgentMessage]
	agentResourceIDCache   *lru.Cache[string, *AgentMessage]
	machineIDCache         *lru.Cache[string, *MachineMessage]
	machineResourceIDCache *lru.Cache[string, *MachineMessage]
	rolesCache             *lru.Cache[string, *RoleMessage]

	// userMcpConfigSetting caches the USER_MCP_CONFIG row for a short TTL so
	// per-call MCP gateway checks do not hit the database. Cleared on upsert.
	userMcpConfigMu       sync.Mutex
	userMcpConfigSetting  *SettingMessage
	userMcpConfigCachedAt time.Time

	// globalMentionIndex caches the unscoped agent/user directory used by
	// authentication/bootstrap paths; tenant-scoped mention projections live in
	// globalMentionIndexes and are keyed by TenantProjectionKey.
	globalMentionIndexMu sync.RWMutex
	globalMentionIndex   *GlobalMentionIndex
	globalMentionIndexes map[string]*GlobalMentionIndex

	// activity worker pool bounds the fire-and-forget activity generation so a
	// message burst cannot spawn an unbounded number of goroutines. Jobs are
	// enqueued without blocking the message-send critical path; when the queue
	// is full (or the store is closing) the activity row is dropped, which is
	// acceptable for this best-effort path.
	activityMu     sync.RWMutex
	activityJobs   chan activityJob
	activityStop   chan struct{}
	activityWg     sync.WaitGroup
	activityClosed bool
}

func New(ctx context.Context, pgURL string, enableCache bool) (*Store, error) {
	userIDCache, err := lru.New[int, *UserMessage](32768)
	if err != nil {
		return nil, err
	}
	userEmailCache, err := lru.New[string, *UserMessage](32768)
	if err != nil {
		return nil, err
	}
	userHandleCache, err := lru.New[string, *UserMessage](32768)
	if err != nil {
		return nil, err
	}

	dbConnManager := NewDBConnectionManager(pgURL)
	if err := dbConnManager.Initialize(ctx); err != nil {
		return nil, err
	}
	settingCache, err := lru.New[models.SettingName, *SettingMessage](32768)
	if err != nil {
		return nil, err
	}
	policyCache, err := lru.New[string, *PolicyMessage](32768)
	if err != nil {
		return nil, err
	}
	idpCache, err := lru.New[string, *IdentityProviderMessage](32768)
	if err != nil {
		return nil, err
	}
	groupCache, err := lru.New[string, *GroupMessage](32768)
	if err != nil {
		return nil, err
	}
	agentIDCache, err := lru.New[string, *AgentMessage](32768)
	if err != nil {
		return nil, err
	}
	agentResourceIDCache, err := lru.New[string, *AgentMessage](32768)
	if err != nil {
		return nil, err
	}
	machineIDCache, err := lru.New[string, *MachineMessage](32768)
	if err != nil {
		return nil, err
	}
	machineResourceIDCache, err := lru.New[string, *MachineMessage](32768)
	if err != nil {
		return nil, err
	}
	rolesCache, err := lru.New[string, *RoleMessage](512)
	if err != nil {
		return nil, err
	}
	s := &Store{
		dbConnManager:          dbConnManager,
		enableCache:            enableCache,
		userIDCache:            userIDCache,
		userEmailCache:         userEmailCache,
		userHandleCache:        userHandleCache,
		settingCache:           settingCache,
		policyCache:            policyCache,
		idpCache:               idpCache,
		groupCache:             groupCache,
		agentIDCache:           agentIDCache,
		agentResourceIDCache:   agentResourceIDCache,
		machineIDCache:         machineIDCache,
		machineResourceIDCache: machineResourceIDCache,
		rolesCache:             rolesCache,
	}
	s.startActivityWorkers()

	return s, nil
}

func (s *Store) Close() error {
	// Stop the activity workers and drain their queue before closing the
	// database, so an in-flight activity write cannot use a closed *sql.DB.
	s.stopActivityWorkers()
	if closer, ok := s.roomNotifier.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	return s.dbConnManager.Close()
}

func (s *Store) GetDB() *sql.DB {
	return s.dbConnManager.GetDB()
}

func getPolicyCacheKey(ctx context.Context, resourceType models.Policy_Resource, resource string, policyType models.Policy_Type) string {
	organizationID := "default"
	if value, ok := common.GetOrganizationIDFromContext(ctx); ok && value != "" {
		organizationID = value
	}
	return TenantCacheKey(organizationID, "policy", fmt.Sprintf("%s/%s/%s", resourceType, resource, policyType))
}
