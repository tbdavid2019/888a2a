package migration

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// latestSQL loads the canonical cumulative schema file. It is the baseline
// applied to fresh installs by the migrator; the tests here guard its contents
// directly rather than executing it against a live database.
func latestSQL(t *testing.T) string {
	t.Helper()
	// This test file lives in backend/manager/migration/, so migration/LATEST.sql
	// is a relative path under it. go test runs with the package directory as the
	// working directory.
	bytes, err := os.ReadFile("migration/LATEST.sql")
	if err != nil {
		t.Fatalf("read migration/LATEST.sql: %v", err)
	}
	return string(bytes)
}

// TestSchemaMigrationHistoryPresent locks in the version-tracking table the
// migrator records applied versions in. It is created by LATEST.sql on fresh
// installs.
func TestSchemaMigrationHistoryPresent(t *testing.T) {
	sql := latestSQL(t)

	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS schema_migration_history") {
		t.Error("LATEST.sql missing schema_migration_history table definition")
	}
	if !strings.Contains(sql, "idx_schema_migration_history_unique_version") {
		t.Error("LATEST.sql missing unique version index on schema_migration_history")
	}
}

// TestSearchChatHistoryTrgmIndexPresent locks in the pg_trgm GIN index that
// makes SearchChatHistory's leading-wildcard `content ILIKE '%q%'` an index
// scan instead of a full table scan. Both the extension and the index must be
// declared idempotently (IF NOT EXISTS) so re-applying the schema is safe.
func TestSearchChatHistoryTrgmIndexPresent(t *testing.T) {
	sql := latestSQL(t)

	if !strings.Contains(sql, "CREATE EXTENSION IF NOT EXISTS pg_trgm") {
		t.Fatal("pg_trgm extension must be created idempotently for trigram ILIKE search")
	}
	if !strings.Contains(sql, "idx_chat_message_content_trgm") {
		t.Fatal("GIN trgm index on chat_message.content is missing; SearchChatHistory would full-scan")
	}
	if !strings.Contains(sql, "gin_trgm_ops") {
		t.Fatal("trgm index must use gin_trgm_ops to serve ILIKE")
	}
	if !strings.Contains(sql, "CREATE INDEX IF NOT EXISTS idx_chat_message_content_trgm") {
		t.Fatal("trgm index must be created with IF NOT EXISTS so re-applying the schema is safe")
	}
	if !strings.Contains(sql, "CREATE INDEX IF NOT EXISTS idx_file_original_name_trgm") {
		t.Fatal("GIN trgm index on file.original_name is missing; attachment-name search would full-scan")
	}
	if !strings.Contains(sql, "CREATE INDEX IF NOT EXISTS idx_chat_message_search_text_trgm") {
		t.Fatal("GIN trgm index on chat_message.search_text is missing; markdown-stripped search would full-scan")
	}
	if !strings.Contains(sql, "chat_occurrences") {
		t.Fatal("chat_occurrences function is missing; term-frequency ranking would fail")
	}
}

// TestUniqueConstraintsPresent locks in the three unique indexes that close
// the T10 race/correctness gaps: at most one direct conversation per
// (agent, user), unique active principal.email, and unique agent_token.token_hash.
// All are declared idempotently.
func TestUniqueConstraintsPresent(t *testing.T) {
	sql := latestSQL(t)

	for _, want := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_dm_unique",
		"ON conversation(agent_id, created_by) WHERE type = 1",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_principal_unique_email",
		"ON principal(email) WHERE deleted = FALSE",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_token_hash ON agent_token(token_hash)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing unique-constraint declaration: %q", want)
		}
	}
	// The DM index must be partial (type = 1) so channels (type=2, agent_id NULL)
	// are not constrained; the email index must be partial (deleted = FALSE) so a
	// soft-deleted address can be reused.
	if !strings.Contains(sql, "DROP INDEX IF EXISTS idx_agent_token_hash") {
		t.Fatal("non-unique idx_agent_token_hash must be dropped before recreating as unique")
	}
}

// TestReminderTablePresent locks in the reminder table that backs scheduled/
// recurring agent tasks. It mirrors task (1:1 with its trigger message, thread
// rooted at it) plus schedule columns (fire_at, cron_expr, tz) and retry-audit
// columns. The assignee is NOT NULL (atomic create+claim). All declarations are
// idempotent so re-applying the schema is safe.
func TestReminderTablePresent(t *testing.T) {
	sql := latestSQL(t)

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS reminder",
		"message_id UUID PRIMARY KEY REFERENCES chat_message(id) ON DELETE CASCADE",
		"conversation_id UUID NOT NULL REFERENCES conversation(id) ON DELETE CASCADE",
		"assignee_agent_id INTEGER NOT NULL REFERENCES agent(id) ON DELETE CASCADE",
		"task_content TEXT NOT NULL",
		"fire_at TIMESTAMPTZ NOT NULL",
		"cron_expr TEXT",
		"tz TEXT NOT NULL DEFAULT 'UTC'",
		"CONSTRAINT reminder_status_check CHECK (status IN (1,2,3,4,5,6))",
		"CREATE INDEX IF NOT EXISTS idx_reminder_assignee_status ON reminder(assignee_agent_id, status)",
		"CREATE INDEX IF NOT EXISTS idx_reminder_fire_at ON reminder(fire_at) WHERE status = 1",
		"CREATE INDEX IF NOT EXISTS idx_reminder_retry ON reminder(next_retry_at) WHERE status = 2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing reminder declaration: %q", want)
		}
	}
}

// TestUserChannelCursorPresent locks in the user_channel_cursor table that backs
// the user-facing unread badge. It mirrors agent_channel_cursor: monotonic
// read_version, cascade deletes, and a missing row treated as caught-up. All
// declarations are idempotent so re-applying the schema is safe.
func TestUserChannelCursorPresent(t *testing.T) {
	sql := latestSQL(t)

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS user_channel_cursor",
		"principal_id INTEGER NOT NULL REFERENCES principal(id) ON DELETE CASCADE",
		"conversation_id UUID NOT NULL REFERENCES conversation(id) ON DELETE CASCADE",
		"read_version BIGINT NOT NULL DEFAULT 0",
		"PRIMARY KEY (principal_id, conversation_id)",
		"CREATE INDEX IF NOT EXISTS idx_user_channel_cursor_user",
		"CREATE INDEX IF NOT EXISTS idx_user_channel_cursor_conv",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing user_channel_cursor declaration: %q", want)
		}
	}
}

// TestAgentDMUniqueIndexPresent locks in the agent-to-agent DM (conversation
// type 3) dedup infrastructure: the ordered agent_dm_a/agent_dm_b columns, the
// order CHECK, and the partial unique index that makes GetOrCreateAgentDM
// race-free, mirroring idx_conversation_dm_unique for type-1 user DMs. All
// declarations are idempotent (IF NOT EXISTS / DO-block guarded) so re-applying
// the schema is safe.
func TestAgentDMUniqueIndexPresent(t *testing.T) {
	sql := latestSQL(t)

	for _, want := range []string{
		"ALTER TABLE conversation ADD COLUMN IF NOT EXISTS agent_dm_a INTEGER REFERENCES agent(id) ON DELETE SET NULL",
		"ALTER TABLE conversation ADD COLUMN IF NOT EXISTS agent_dm_b INTEGER REFERENCES agent(id) ON DELETE SET NULL",
		"conversation_agent_dm_order_check",
		"agent_dm_a < agent_dm_b",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_agent_dm_unique",
		"ON conversation(agent_dm_a, agent_dm_b) WHERE type = 3",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing agent-DM declaration: %q", want)
		}
	}
}

// TestChannelTitleUniqueIndexPresent locks in the unique partial index on
// conversation.title for channels (type=2) so a "#<title>" address resolves to
// exactly one conversation. Pre-launch, so no backfill is needed. Idempotent.
func TestChannelTitleUniqueIndexPresent(t *testing.T) {
	sql := latestSQL(t)

	for _, want := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_channel_title_unique",
		"ON conversation(title) WHERE type = 2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing channel-title unique index declaration: %q", want)
		}
	}
}

// TestCommandTokenUsageTablePresent locks in the standalone token-usage table
// that keeps per-command token aggregates cheap. One row per command
// (command_id UNIQUE) with dimension columns (agent_id/principal_id/
// machine_id) denormalized from command so agent/principal/machine + time
// aggregation needs no join. All declarations are idempotent so re-applying
// the schema is safe.
func TestCommandTokenUsageTablePresent(t *testing.T) {
	sql := latestSQL(t)

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS command_token_usage",
		"command_id UUID NOT NULL UNIQUE REFERENCES command(id) ON DELETE CASCADE",
		"agent_id INTEGER NOT NULL REFERENCES agent(id) ON DELETE CASCADE",
		"principal_id INTEGER NOT NULL REFERENCES principal(id)",
		"machine_id INTEGER REFERENCES machine(id) ON DELETE SET NULL",
		"total_tokens BIGINT NOT NULL DEFAULT 0",
		"CREATE INDEX IF NOT EXISTS idx_command_token_usage_agent_time",
		"CREATE INDEX IF NOT EXISTS idx_command_token_usage_principal_time",
		"CREATE INDEX IF NOT EXISTS idx_command_token_usage_machine_time",
		"CREATE INDEX IF NOT EXISTS idx_command_token_usage_time",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing command_token_usage declaration: %q", want)
		}
	}
}

// TestA2AWorkPersistencePresent locks in the additive fresh-install and
// upgrade paths for the A2A-compatible work model. The cumulative baseline
// and the incremental migration must expose the same durable tables and
// tenant-scoped idempotency contract. Full PostgreSQL execution is covered by
// the gated migrator integration tests.
func TestA2AWorkPersistencePresent(t *testing.T) {
	latest := latestSQL(t)
	incrementalBytes, err := os.ReadFile("migration/1.1/0026##a2a-work-persistence.sql")
	if err != nil {
		t.Fatalf("read A2A work incremental migration: %v", err)
	}
	incremental := string(incrementalBytes)

	for _, table := range []string{
		"a2a888_work_context",
		"a2a888_work",
		"a2a888_work_artifact",
		"a2a888_work_event",
	} {
		declaration := "CREATE TABLE IF NOT EXISTS " + table
		if !strings.Contains(latest, declaration) {
			t.Errorf("LATEST.sql missing %s", declaration)
		}
		if !strings.Contains(incremental, declaration) {
			t.Errorf("incremental migration missing %s", declaration)
		}
	}

	for _, want := range []string{
		"PRIMARY KEY (tenant_id, work_id)",
		"a2a_task_id TEXT NOT NULL",
		"source_conversation_id UUID REFERENCES conversation(id)",
		"source_task_id UUID REFERENCES task(message_id)",
		"AUTH_REQUIRED",
		"SUBMITTED",
		"WORKING",
		"INPUT_REQUIRED",
		"COMPLETED",
		"FAILED",
		"CANCELED",
		"REJECTED",
		"uq_a2a888_work_idempotency",
		"ON a2a888_work (tenant_id, requester_agent_id, idempotency_key)",
		"uq_a2a888_work_a2a_task_id",
		"parent_work_id",
		"max_runtime_ms",
		"used_work_units",
		"file_id UUID REFERENCES file(id)",
		"root_trace_id",
		"sequence BIGINT NOT NULL",
		"a2a888_work_event_sequence_check CHECK (sequence > 0)",
		"uq_a2a888_work_event_work_sequence",
		"ON a2a888_work_event (tenant_id, work_id, sequence)",
		"metadata JSONB NOT NULL DEFAULT '{}'",
	} {
		if !strings.Contains(latest, want) {
			t.Errorf("LATEST.sql missing A2A work persistence contract %q", want)
		}
		if !strings.Contains(incremental, want) {
			t.Errorf("incremental migration missing A2A work persistence contract %q", want)
		}
	}

	if strings.Contains(incremental, "DROP TABLE") || strings.Contains(incremental, "TRUNCATE") {
		t.Fatal("A2A work upgrade migration must be additive")
	}

	bodyStart := strings.Index(incremental, "CREATE TABLE IF NOT EXISTS a2a888_work_context")
	if bodyStart < 0 {
		t.Fatal("incremental migration has no canonical A2A work DDL body")
	}
	if !strings.Contains(latest, incremental[bodyStart:]) {
		t.Fatal("LATEST.sql A2A work DDL is out of sync with the incremental migration")
	}
}

// TestOrganizationTenancySchemaPresent verifies the multi-tenant schema in both
// LATEST.sql (fresh installs) and 0028##organization-tenancy.sql (upgrades).
func TestOrganizationTenancySchemaPresent(t *testing.T) {
	latest := latestSQL(t)
	incBytes, err := os.ReadFile("migration/1.1/0028##organization-tenancy.sql")
	if err != nil {
		t.Fatalf("read 0028##organization-tenancy.sql: %v", err)
	}
	incremental := string(incBytes)

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS organizations",
		"CREATE TABLE IF NOT EXISTS workspaces",
		"CREATE TABLE IF NOT EXISTS organization_memberships",
		"idx_organizations_slug",
		"idx_workspaces_organization",
		"idx_org_memberships_principal",
		"organizations_state_check CHECK (state IN ('ACTIVE', 'SUSPENDED', 'CLOSED'))",
		"org_memberships_role_check CHECK (role IN ('OWNER', 'ADMIN', 'MEMBER', 'GUEST'))",
		"org_memberships_state_check CHECK (state IN ('ACTIVE', 'SUSPENDED', 'INVITED'))",
		"idx_agent_organization",
		"idx_agent_workspace",
		"idx_machine_organization",
		"idx_conversation_organization",
		"idx_conversation_workspace",
		"idx_mcp_server_organization",
		"idx_file_organization",
		"idx_task_organization",
		"idx_audit_log_organization",
		"idx_api_provider_organization",
		"idx_user_group_organization",
		"idx_reminder_organization",
		"REFERENCES organizations(id)",
		"REFERENCES workspaces(id)",
	} {
		if !strings.Contains(latest, want) {
			t.Errorf("LATEST.sql missing Organization tenancy contract %q", want)
		}
		if !strings.Contains(incremental, want) {
			t.Errorf("incremental migration missing Organization tenancy contract %q", want)
		}
	}

	if strings.Contains(incremental, "DROP TABLE") || strings.Contains(incremental, "TRUNCATE") {
		t.Fatal("Organization tenancy upgrade migration must be additive")
	}
}

// TestDefaultOrganizationMigration_ExistingDeployments verifies default tenant backfill
// for existing principals, agents, machines, and conversations (Task 2.3).
func TestDefaultOrganizationMigration_ExistingDeployments(t *testing.T) {
	latest := latestSQL(t)
	incBytes, err := os.ReadFile("migration/1.1/0028##organization-tenancy.sql")
	if err != nil {
		t.Fatalf("read 0028##organization-tenancy.sql: %v", err)
	}
	incremental := string(incBytes)

	seeds := []string{
		"INSERT INTO organizations (id, name, slug, state)",
		"VALUES ('default', 'Default Organization', 'default', 'ACTIVE')",
		"ON CONFLICT (id) DO NOTHING",
		"INSERT INTO workspaces (id, organization_id, name, slug, is_default)",
		"VALUES ('default', 'default', 'Default Workspace', 'default', true)",
		"INSERT INTO organization_memberships (organization_id, principal_id, role, state, workspace_ids)",
		"SELECT 'default', id, 'OWNER', 'ACTIVE', ARRAY['default']",
		"FROM principal",
		"ON CONFLICT (organization_id, principal_id) DO NOTHING",
		"ALTER TABLE principal ADD COLUMN IF NOT EXISTS default_organization_id TEXT DEFAULT 'default' REFERENCES organizations(id)",
	}

	for _, stmt := range seeds {
		if !strings.Contains(latest, stmt) {
			t.Errorf("LATEST.sql missing default tenant migration statement: %q", stmt)
		}
		if !strings.Contains(incremental, stmt) {
			t.Errorf("0028 migration missing default tenant migration statement: %q", stmt)
		}
	}
}

// TestCollaborationResourcesTenantColumnsAndIndexes verifies tenant columns, foreign keys,
// and indexes across all collaboration entities (Task 2.4).
func TestCollaborationResourcesTenantColumnsAndIndexes(t *testing.T) {
	latest := latestSQL(t)
	incBytes, err := os.ReadFile("migration/1.1/0028##organization-tenancy.sql")
	if err != nil {
		t.Fatalf("read 0028##organization-tenancy.sql: %v", err)
	}
	incremental := string(incBytes)

	type columnCheck struct {
		table   string
		column  string
		fkTable string
		index   string
	}

	checks := []columnCheck{
		{table: "principal", column: "default_organization_id", fkTable: "organizations", index: ""},
		{table: "agent", column: "organization_id", fkTable: "organizations", index: "idx_agent_organization"},
		{table: "agent", column: "workspace_id", fkTable: "workspaces", index: "idx_agent_workspace"},
		{table: "machine", column: "organization_id", fkTable: "organizations", index: "idx_machine_organization"},
		{table: "conversation", column: "organization_id", fkTable: "organizations", index: "idx_conversation_organization"},
		{table: "conversation", column: "workspace_id", fkTable: "workspaces", index: "idx_conversation_workspace"},
		{table: "mcp_server", column: "organization_id", fkTable: "organizations", index: "idx_mcp_server_organization"},
		{table: "file", column: "organization_id", fkTable: "organizations", index: "idx_file_organization"},
		{table: "task", column: "organization_id", fkTable: "organizations", index: "idx_task_organization"},
		{table: "audit_log", column: "organization_id", fkTable: "organizations", index: "idx_audit_log_organization"},
		{table: "api_provider", column: "organization_id", fkTable: "organizations", index: "idx_api_provider_organization"},
		{table: "user_group", column: "organization_id", fkTable: "organizations", index: "idx_user_group_organization"},
		{table: "reminder", column: "organization_id", fkTable: "organizations", index: "idx_reminder_organization"},
	}

	for _, c := range checks {
		colPattern := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s", c.table, c.column)
		if !strings.Contains(latest, colPattern) {
			t.Errorf("LATEST.sql missing column DDL: %q", colPattern)
		}
		if !strings.Contains(incremental, colPattern) {
			t.Errorf("0028 migration missing column DDL: %q", colPattern)
		}

		fkPattern := fmt.Sprintf("REFERENCES %s(id)", c.fkTable)
		if !strings.Contains(latest, fkPattern) {
			t.Errorf("LATEST.sql missing foreign key reference to %s", c.fkTable)
		}
		if !strings.Contains(incremental, fkPattern) {
			t.Errorf("0028 migration missing foreign key reference to %s", c.fkTable)
		}

		if c.index != "" {
			idxPattern := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s(%s)", c.index, c.table, c.column)
			if !strings.Contains(latest, idxPattern) {
				t.Errorf("LATEST.sql missing index DDL: %q", idxPattern)
			}
			if !strings.Contains(incremental, idxPattern) {
				t.Errorf("0028 migration missing index DDL: %q", idxPattern)
			}
		}
	}
}

func TestOrganizationIAMMigrationPresent(t *testing.T) {
	latest := latestSQL(t)
	incBytes, err := os.ReadFile("migration/1.1/0032##organization-iam-and-audit.sql")
	if err != nil {
		t.Fatalf("read organization IAM migration: %v", err)
	}
	incremental := string(incBytes)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS organization_membership_workspaces",
		"CREATE TABLE IF NOT EXISTS organization_group_bindings",
		"idx_org_group_bindings_unique",
		"organization_group_binding_group_fk",
		"organization_group_binding_workspace_fk",
		"idx_policy_tenant_resource_type",
		"idx_role_tenant_resource_id",
		"requester_id TEXT NOT NULL DEFAULT ''",
		"executor_id TEXT NOT NULL DEFAULT ''",
		"org_memberships_role_check",
		"BILLING_ADMIN",
		"AGENT_ADMIN",
		"APPROVER",
	} {
		if !strings.Contains(latest, want) {
			t.Errorf("LATEST.sql missing organization IAM contract %q", want)
		}
		if !strings.Contains(incremental, want) {
			t.Errorf("0032 migration missing organization IAM contract %q", want)
		}
	}
	if strings.Contains(incremental, "DROP TABLE") || strings.Contains(incremental, "TRUNCATE") {
		t.Fatal("organization IAM upgrade migration must be additive")
	}
}

func TestCollaborationProjectionTenantColumnsPresent(t *testing.T) {
	latest := latestSQL(t)
	incBytes, err := os.ReadFile("migration/1.1/0032##organization-iam-and-audit.sql")
	if err != nil {
		t.Fatalf("read organization IAM migration: %v", err)
	}
	incremental := string(incBytes)
	for _, table := range []string{
		"command", "chat_message", "conversation_member_meta", "command_conversation",
		"command_token_usage", "agent_channel_cursor", "user_channel_cursor",
		"thread_participant", "message_reaction",
	} {
		want := "ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS organization_id"
		if !strings.Contains(latest, want) || !strings.Contains(incremental, want) {
			t.Errorf("tenant projection column missing for %s", table)
		}
	}
	for _, want := range []string{
		"UPDATE chat_message cm SET organization_id = conv.organization_id",
		"UPDATE command c SET organization_id = conv.organization_id",
		"idx_chat_message_organization",
		"idx_command_conversation_organization",
	} {
		if !strings.Contains(latest, want) || !strings.Contains(incremental, want) {
			t.Errorf("tenant projection backfill/index missing %q", want)
		}
	}
}

func TestDurableOutboxSchemaPresent(t *testing.T) {
	latest := latestSQL(t)
	incBytes, err := os.ReadFile("migration/1.1/0029##durable-outbox.sql")
	if err != nil {
		t.Fatalf("read 0029##durable-outbox.sql: %v", err)
	}
	incremental := string(incBytes)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS a2a888_outbox_event",
		"organization_id TEXT NOT NULL REFERENCES organizations(id)",
		"idempotency_key TEXT NOT NULL DEFAULT ''",
		"status TEXT NOT NULL DEFAULT 'PENDING'",
		"claim_expires_at TIMESTAMPTZ",
		"uq_a2a888_outbox_event_idempotency",
		"idx_a2a888_outbox_event_claimable",
	} {
		if !strings.Contains(latest, want) {
			t.Errorf("LATEST.sql missing outbox contract %q", want)
		}
		if !strings.Contains(incremental, want) && want != "FOR UPDATE SKIP LOCKED" {
			t.Errorf("incremental migration missing outbox contract %q", want)
		}
	}
}

func TestDurableConnectorInboxSchemaPresent(t *testing.T) {
	latest := latestSQL(t)
	incBytes, err := os.ReadFile("migration/1.1/0030##connector-inbox.sql")
	if err != nil {
		t.Fatalf("read 0030##connector-inbox.sql: %v", err)
	}
	incremental := string(incBytes)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS a2a888_connector_inbox",
		"PRIMARY KEY (organization_id, installation_id, external_event_id)",
		"a2a888_connector_inbox_status_check",
		"idx_a2a888_connector_inbox_pending",
	} {
		if !strings.Contains(latest, want) || !strings.Contains(incremental, want) {
			t.Errorf("connector inbox schema missing %q", want)
		}
	}
}

func TestOutboxReconciliationSchemaPresent(t *testing.T) {
	latest := latestSQL(t)
	incBytes, err := os.ReadFile("migration/1.1/0031##outbox-reconciliation.sql")
	if err != nil {
		t.Fatalf("read 0031##outbox-reconciliation.sql: %v", err)
	}
	incremental := string(incBytes)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS a2a888_outbox_reconciliation",
		"event_id TEXT NOT NULL REFERENCES a2a888_outbox_event(event_id)",
		"action IN ('REPLAY', 'RECONCILE')",
		"idx_a2a888_outbox_reconciliation_tenant",
	} {
		if !strings.Contains(latest, want) || !strings.Contains(incremental, want) {
			t.Errorf("outbox reconciliation schema missing %q", want)
		}
	}
}
