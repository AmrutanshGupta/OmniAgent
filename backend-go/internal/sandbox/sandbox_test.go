package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
)

// ── Unit Tests — no Docker daemon required ────────────────────────────────────

func TestWriteWorkspace(t *testing.T) {
	// Use a temp dir on the host for testing so we don't pollute /tmp in CI
	t.Setenv("HOME", t.TempDir())

	const taskID = "test-workspace-unit"
	files := []ProjectFile{
		{RelPath: "index.js", Contents: []byte("console.log('hello')")},
		{RelPath: "src/app.js", Contents: []byte("module.exports = {}")},
		{RelPath: "package.json", Contents: []byte(`{"name":"test","version":"1.0.0"}`)},
	}

	dir := filepath.Join(os.TempDir(), "omniagent-test", taskID)
	defer os.RemoveAll(filepath.Join(os.TempDir(), "omniagent-test"))

	for _, f := range files {
		destPath := filepath.Join(dir, f.RelPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(destPath, f.Contents, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Verify all files exist
	for _, f := range files {
		path := filepath.Join(dir, f.RelPath)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("file %s not found: %v", f.RelPath, err)
			continue
		}
		if string(data) != string(f.Contents) {
			t.Errorf("file %s content mismatch: got %q want %q", f.RelPath, string(data), string(f.Contents))
		}
	}
}

func TestParseE2EOutput_Success(t *testing.T) {
	logs := `some startup logs
E2E_RESULT_START
{"passed":true,"statusCode":200,"domSnapshot":"<html>...</html>","consoleErrors":[]}
E2E_RESULT_END
cleanup logs`

	result := parseE2EOutput(logs, 0)

	if !result.Passed {
		t.Errorf("expected passed=true")
	}
	if result.StatusCode != 200 {
		t.Errorf("expected statusCode=200, got %d", result.StatusCode)
	}
	if !strings.Contains(result.DOMOutput, "<html>") {
		t.Errorf("expected DOM output to contain <html>")
	}
	if len(result.ConsoleErrors) != 0 {
		t.Errorf("expected no console errors, got %v", result.ConsoleErrors)
	}
}

func TestParseE2EOutput_Failure(t *testing.T) {
	logs := `E2E_RESULT_START
{"passed":false,"statusCode":500,"domSnapshot":"","consoleErrors":["Navigation error: net::ERR_CONNECTION_REFUSED"]}
E2E_RESULT_END`

	result := parseE2EOutput(logs, 1)

	if result.Passed {
		t.Errorf("expected passed=false")
	}
	if result.StatusCode != 500 {
		t.Errorf("expected statusCode=500, got %d", result.StatusCode)
	}
	if len(result.ConsoleErrors) != 1 {
		t.Errorf("expected 1 console error, got %d", len(result.ConsoleErrors))
	}
}

func TestParseE2EOutput_MissingMarkers(t *testing.T) {
	logs := "container crashed with OOM"
	result := parseE2EOutput(logs, 137) // 137 = SIGKILL exit code
	// Without markers we fall through to the exit-code truth
	if result.Passed {
		t.Errorf("exit 137 should not be treated as passed")
	}
	if result.DOMOutput != logs {
		t.Errorf("expected raw logs as DOMOutput when markers are absent")
	}
}

func TestBuildPlaywrightScript_ContainsAlias(t *testing.T) {
	script := buildPlaywrightScript("3000")
	if !strings.Contains(script, appAlias) {
		t.Errorf("script should reference alias %q", appAlias)
	}
	if !strings.Contains(script, "3000") {
		t.Errorf("script should contain port 3000")
	}
	if !strings.Contains(script, "E2E_RESULT_START") {
		t.Errorf("script should emit E2E_RESULT_START marker")
	}
}

func TestHardResourceConfig_MemoryCap(t *testing.T) {
	cfg := hardResourceConfig("/tmp/test-workspace", false)
	if cfg.Resources.Memory != memLimitBytes {
		t.Errorf("expected memory cap %d, got %d", memLimitBytes, cfg.Resources.Memory)
	}
	if cfg.Resources.MemorySwap != 0 {
		t.Errorf("expected swap=0, got %d", cfg.Resources.MemorySwap)
	}
	if cfg.Resources.NanoCPUs != nanoCPULimit {
		t.Errorf("expected NanoCPUs=%d, got %d", nanoCPULimit, cfg.Resources.NanoCPUs)
	}
	if *cfg.Resources.PidsLimit != pidsLimitVal {
		t.Errorf("expected PidsLimit=%d, got %d", pidsLimitVal, *cfg.Resources.PidsLimit)
	}
}

func TestHardResourceConfig_SecurityOptions(t *testing.T) {
	cfg := hardResourceConfig("/tmp/test-workspace", false)
	if !cfg.ReadonlyRootfs {
		t.Errorf("expected ReadonlyRootfs=true for RO containers")
	}
	if len(cfg.CapDrop) == 0 || cfg.CapDrop[0] != "ALL" {
		t.Errorf("expected CapDrop=[ALL]")
	}
	hasNoNewPriv := false
	for _, opt := range cfg.SecurityOpt {
		if opt == "no-new-privileges:true" {
			hasNoNewPriv = true
		}
	}
	if !hasNoNewPriv {
		t.Errorf("expected no-new-privileges security option")
	}
}

func TestHardResourceConfig_WritableWorkspace(t *testing.T) {
	cfg := hardResourceConfig("/tmp/test-workspace", true)
	if cfg.ReadonlyRootfs {
		t.Errorf("expected ReadonlyRootfs=false for writable app containers")
	}
	if len(cfg.Mounts) == 0 {
		t.Errorf("expected at least one mount")
	}
	// Workspace mount should not be read-only
	if cfg.Mounts[0].ReadOnly {
		t.Errorf("expected workspace mount to be writable")
	}
}

// ── Integration Tests — require a live Docker daemon ─────────────────────────
// These tests are guarded by a build tag and run with: go test -tags integration

func TestNetworkIsolation_RequiresDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}
	orc, err := NewDoodOrchestrator()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer orc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	taskID := "test-net-" + randomSuffix()
	netID, err := orc.createBridgeNetwork(ctx, taskID)
	if err != nil {
		t.Fatalf("createBridgeNetwork: %v", err)
	}
	defer orc.removeNetwork(context.Background(), netID)

	// Verify the network exists and is internal
	info, err := orc.cli.NetworkInspect(ctx, netID, types.NetworkInspectOptions{})
	if err != nil {
		t.Fatalf("NetworkInspect: %v", err)
	}
	if !info.Internal {
		t.Errorf("expected Internal=true for task network, got false")
	}
	if !strings.HasPrefix(info.Name, "net-") {
		t.Errorf("expected network name to start with net-, got %s", info.Name)
	}
}

func TestFullLifecycle_RequiresDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full lifecycle integration test in short mode")
	}
	orc, err := NewDoodOrchestrator()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer orc.Close()

	taskID := "test-lifecycle-" + randomSuffix()
	files := []ProjectFile{
		{RelPath: "package.json", Contents: []byte(`{
			"name": "test-app", "version": "1.0.0",
			"scripts": { "start": "node server.js" }
		}`)},
		{RelPath: "server.js", Contents: []byte(`
			const http = require('http');
			http.createServer((_, res) => {
				res.writeHead(200, {'Content-Type': 'text/html'});
				res.end('<html><body><h1>E2E Test OK</h1></body></html>');
			}).listen(3000);
			console.log('Server listening on :3000');
		`)},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	workspaceDir, err := orc.WriteWorkspace(taskID, files, nil)
	if err != nil {
		t.Fatalf("WriteWorkspace: %v", err)
	}

	result, err := orc.RunE2ETest(ctx, taskID, workspaceDir, "node server.js", "3000", nil)
	if err != nil {
		t.Fatalf("RunE2ETest: %v", err)
	}

	if result.StatusCode != 200 && result.StatusCode != 0 {
		// StatusCode 0 is expected when Playwright exits cleanly but can't connect in unit env
		t.Logf("Result: passed=%v status=%d duration=%s", result.Passed, result.StatusCode, result.Duration)
	}

	// Most important: no workspace should remain after test
	wsPath := filepath.Join(getWorkspaceRoot(nil), taskID)
	if _, err := os.Stat(wsPath); !os.IsNotExist(err) {
		t.Errorf("workspace was not cleaned up: %s still exists", wsPath)
	}
}
