package store

import (
	"strings"
	"testing"
)

func TestWorkSQLQueries(t *testing.T) {
	t.Run("insertWorkContextSQL", func(t *testing.T) {
		if !strings.Contains(insertWorkContextSQL, "ON CONFLICT (tenant_id, context_id) DO UPDATE") {
			t.Fatal("insertWorkContextSQL must handle conflicts on (tenant_id, context_id)")
		}
		if !strings.Contains(insertWorkContextSQL, "RETURNING") {
			t.Fatal("insertWorkContextSQL must return inserted context record")
		}
	})

	t.Run("insertWorkSQL", func(t *testing.T) {
		if !strings.Contains(insertWorkSQL, "INSERT INTO a2a888_work") {
			t.Fatal("insertWorkSQL must target a2a888_work table")
		}
		if !strings.Contains(insertWorkSQL, "idempotency_key") {
			t.Fatal("insertWorkSQL must persist idempotency_key")
		}
	})

	t.Run("updateWorkStateSQL", func(t *testing.T) {
		if !strings.Contains(updateWorkStateSQL, "version = version + 1") {
			t.Fatal("updateWorkStateSQL must increment optimistic locking version")
		}
		if !strings.Contains(updateWorkStateSQL, "version = $5") {
			t.Fatal("updateWorkStateSQL must check expected version")
		}
		if !strings.Contains(updateWorkStateSQL, "COMPLETED") || !strings.Contains(updateWorkStateSQL, "FAILED") {
			t.Fatal("updateWorkStateSQL must handle terminal states for completed_at")
		}
	})

	t.Run("insertWorkArtifactSQL", func(t *testing.T) {
		if !strings.Contains(insertWorkArtifactSQL, "ON CONFLICT (tenant_id, work_id, artifact_id) DO UPDATE") {
			t.Fatal("insertWorkArtifactSQL must handle upserts on (tenant_id, work_id, artifact_id)")
		}
	})

	t.Run("insertWorkEventSQL", func(t *testing.T) {
		if !strings.Contains(insertWorkEventSQL, "INSERT INTO a2a888_work_event") {
			t.Fatal("insertWorkEventSQL must target a2a888_work_event table")
		}
		if !strings.Contains(insertWorkEventSQL, "sequence") {
			t.Fatal("insertWorkEventSQL must insert monotonic sequence")
		}
	})

	t.Run("listWorkEventsSQL", func(t *testing.T) {
		if !strings.Contains(listWorkEventsSQL, "ORDER BY sequence ASC") {
			t.Fatal("listWorkEventsSQL must order by sequence ASC")
		}
	})
}
