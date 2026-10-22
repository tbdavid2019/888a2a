//go:build unix

package supervisor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tbdavid2019/888a2a/backend/agent/home"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pgrepToken reports whether any running process has token in its command line.
func pgrepToken(t *testing.T, token string) bool {
	t.Helper()
	out, err := exec.Command("pgrep", "-f", token).Output()
	if err == nil {
		return len(out) > 0
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false // pgrep exits 1 when no process matches
	}
	t.Fatalf("pgrep %q failed: %v", token, err)
	return false
}

func TestMain(m *testing.M) {
	if os.Getenv("LAELIA_TEST_WORKER_SLEEP") == "1" {
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestStopDoesNotOrphanWorker is a regression test for the shutdown race where
// `laelia-machine stop` could stop the supervisor (daemon) while watchWorker
// respawned a fresh `run` worker in the gap between stopWorker and
// close(stopWatcher). The respawned worker was then orphaned (reparented to
// init) and kept running under a new pid. After the fix, shutdown sets
// `stopping` and closes stopWatcher before killing the worker, so no respawn
// occurs and exactly zero worker processes remain.
//
// It runs several iterations because the original race was timing-dependent;
// the fix must leave no orphan on every iteration.
func TestStopDoesNotOrphanWorker(t *testing.T) {
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep not available")
	}

	selfExe, err := os.Executable()
	require.NoError(t, err)

	tmp := t.TempDir()
	t.Setenv(home.EnvDir, tmp) // isolate the supervisor addr file + logs
	t.Setenv("LAELIA_TEST_WORKER_SLEEP", "1")

	// Symlink the test binary itself rather than /bin/sleep: on Ubuntu 24.04+
	// (coreutils-from-uutils), /bin/sleep is a multicall binary that checks
	// basename(argv[0]) and immediately exits with code 1 when invoked via a
	// symlink named "laelia-test-worker-*". Re-invoking the Go test binary with
	// LAELIA_TEST_WORKER_SLEEP=1 gives a real process whose argv carries token.
	token := "laelia-test-worker-" + filepath.Base(tmp)
	workerExe := filepath.Join(tmp, token)
	require.NoError(t, os.Symlink(selfExe, workerExe))

	const iterations = 10
	for i := 0; i < iterations; i++ {
		sup, err := New(workerExe, []string{token}, []string{"daemon"}, false)
		require.NoError(t, err, "iter %d: New", i)

		ctx, cancel := context.WithCancel(context.Background())
		runDone := make(chan error, 1)
		go func() { runDone <- sup.Run(ctx) }()

		// Wait for the worker to come up.
		require.Eventually(t, func() bool { return pgrepToken(t, token) },
			5*time.Second, 30*time.Millisecond, "iter %d: worker never started", i)

		// Trigger the real shutdown path that `laelia-machine stop` uses.
		require.NoError(t, Stop(), "iter %d: Stop", i)

		select {
		case err := <-runDone:
			assert.NoError(t, err, "iter %d: Run returned error", i)
		case <-time.After(20 * time.Second):
			cancel()
			t.Fatalf("iter %d: supervisor did not stop within 20s", i)
		}
		cancel()

		// Give any racing respawn a moment to surface in the process table, then
		// assert no worker is left. On the buggy code a respawned worker survived
		// here under a new pid.
		time.Sleep(200 * time.Millisecond)
		if pgrepToken(t, token) {
			_ = exec.Command("pkill", "-f", token).Run()
			t.Fatalf("iter %d: orphan worker still running after stop (shutdown race regression)", i)
		}
	}
}
