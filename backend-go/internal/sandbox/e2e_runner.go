// Package sandbox — E2E multi-container test runner.
// Orchestrates a dual-container topology (App Server + Headless Playwright Tester)
// on an isolated internal Docker bridge network, collects structured results,
// and guarantees full cleanup via deferred teardown.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"github.com/rs/zerolog/log"
	"strings"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	imageAppNode       = "node:20-alpine"
	imageE2EPlaywright = "mcr.microsoft.com/playwright:v1.40.0-focal"
	defaultAppPort     = "3000"
	appAlias           = "app-under-test"
)

// E2EResult captures the outcome of a full end-to-end test run.
type E2EResult struct {
	Passed        bool          `json:"passed"`
	StatusCode    int           `json:"status_code"`
	DOMOutput     string        `json:"dom_output"`
	ConsoleErrors []string      `json:"console_errors"`
	Logs          string        `json:"logs"`
	Duration      time.Duration `json:"duration_ms"`
}

// RunE2ETest provisions the dual-container E2E topology for the given task,
// executes Playwright verification, and guarantees full teardown.
//
//   - taskID: unique per invocation (used for network/container names)
//   - workspaceDir: absolute host path written by WriteWorkspace
//   - appCmd: shell command to install deps and start the server
//   - port: the port the app listens on inside the container
func (o *DoodOrchestrator) RunE2ETest(
	ctx context.Context,
	taskID string,
	workspaceDir string,
	appCmd string,
	port string,
	cfg *config.Config,
) (*E2EResult, error) {
	if port == "" {
		port = defaultAppPort
	}

	start := time.Now()
	log.Printf("[sandbox][e2e][%s] starting E2E run", taskID)

	// ── 1. Pre-pull images ────────────────────────────────────────────────────
	// Use an extended timeout context so cold image pulls don't starve test execution
	pullCtx, pullCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer pullCancel()
	for _, img := range []string{imageAppNode, imageE2EPlaywright} {
		if err := o.pullImageIfAbsent(pullCtx, img); err != nil {
			return nil, fmt.Errorf("sandbox: pull image %s: %w", img, err)
		}
	}

	// ── 2. Hard 30-second execution deadline for running the test containers ──
	execCtx, execCancel := context.WithTimeout(ctx, hardTimeout)
	defer execCancel()

	// ── Isolated internal bridge network ─────────────────────────────────────
	netID, err := o.createBridgeNetwork(execCtx, taskID)
	if err != nil {
		return nil, err
	}
	defer func() {
		bg := context.Background()
		o.removeNetwork(bg, netID)
		RemoveWorkspace(taskID, cfg)
	}()

	// ── 3. Container A — Application Server ──────────────────────────────────
	appID, err := o.createAppContainer(execCtx, taskID, netID, workspaceDir, appCmd, port)
	if err != nil {
		return nil, fmt.Errorf("sandbox: create app container: %w", err)
	}
	defer func() { o.stopAndRemoveContainer(appID) }()

	if err := o.cli.ContainerStart(execCtx, appID, types.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("sandbox: start app container: %w", err)
	}
	log.Printf("[sandbox][e2e][%s] app container started (%s)", taskID, appID[:12])

	// Brief startup grace period — production should poll /health with retry
	select {
	case <-execCtx.Done():
		return nil, execCtx.Err()
	case <-time.After(3 * time.Second):
	}

	// ── 4. Container B — Headless Playwright Tester ──────────────────────────
	script := buildPlaywrightScript(port)
	e2eID, err := o.createTesterContainer(execCtx, taskID, netID, script)
	if err != nil {
		return nil, fmt.Errorf("sandbox: create tester container: %w", err)
	}
	defer func() { o.stopAndRemoveContainer(e2eID) }()

	if err := o.cli.ContainerStart(execCtx, e2eID, types.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("sandbox: start tester container: %w", err)
	}
	log.Printf("[sandbox][e2e][%s] tester container started (%s)", taskID, e2eID[:12])

	// ── 5. Await tester completion and collect output ─────────────────────────
	logs, exitCode, err := o.collectContainerOutput(execCtx, e2eID)
	if err != nil {
		return nil, fmt.Errorf("sandbox: collect tester output: %w", err)
	}

	// ── 6. Parse and return structured results ────────────────────────────────
	result := parseE2EOutput(logs, exitCode)
	result.Duration = time.Since(start)
	log.Printf("[sandbox][e2e][%s] done in %s — passed=%v status=%d",
		taskID, result.Duration.Round(time.Millisecond), result.Passed, result.StatusCode)

	return result, nil
}

// createAppContainer configures Container A: the application server.
// The workspace is mounted writable so npm install can write node_modules.
func (o *DoodOrchestrator) createAppContainer(
	ctx context.Context,
	taskID, netID, workspaceDir, appCmd, port string,
) (string, error) {
	netName := "net-" + taskID
	hostCfg := hardResourceConfig(workspaceDir, true)

	resp, err := o.cli.ContainerCreate(ctx,
		&dockercontainer.Config{
			Image:      imageAppNode,
			WorkingDir: "/workspace",
			Cmd:        []string{"sh", "-c", appCmd},
			Env: []string{
				"NODE_ENV=production",
				"PORT=" + port,
				"npm_config_cache=/tmp/.npm",
			},
			Labels: map[string]string{
				"omniagent.task_id": taskID,
				"omniagent.role":    "app-server",
			},
		},
		hostCfg,
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				netName: networkEndpoint(netID, appAlias),
			},
		},
		nil,
		"omni-app-"+taskID[:8]+"-"+randomSuffix(),
	)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// createTesterContainer configures Container B: the headless Playwright runner.
// The inline script is passed via CMD — no writable workspace needed.
func (o *DoodOrchestrator) createTesterContainer(
	ctx context.Context,
	taskID, netID, script string,
) (string, error) {
	netName := "net-" + taskID
	pidLimit := pidsLimitVal

	resp, err := o.cli.ContainerCreate(ctx,
		&dockercontainer.Config{
			Image: imageE2EPlaywright,
			Cmd:   []string{"node", "-e", script},
			Labels: map[string]string{
				"omniagent.task_id": taskID,
				"omniagent.role":    "e2e-tester",
			},
		},
		&dockercontainer.HostConfig{
			Resources: dockercontainer.Resources{
				Memory:     memLimitBytes,
				MemorySwap: 0,
				NanoCPUs:   nanoCPULimit,
				PidsLimit:  &pidLimit,
			},
			// Playwright needs /tmp for browser temp files
			Tmpfs:       map[string]string{"/tmp": "size=256m,mode=1777"},
			CapDrop:     []string{"ALL"},
			SecurityOpt: []string{"no-new-privileges:true"},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				netName: networkEndpoint(netID),
			},
		},
		nil,
		"omni-e2e-"+taskID[:8]+"-"+randomSuffix(),
	)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// collectContainerOutput blocks until the container exits then returns all logs.
func (o *DoodOrchestrator) collectContainerOutput(ctx context.Context, containerID string) (string, int, error) {
	statusCh, errCh := o.cli.ContainerWait(ctx, containerID, dockercontainer.WaitConditionNotRunning)

	var exitCode int
	select {
	case <-ctx.Done():
		return "", -1, ctx.Err()
	case err := <-errCh:
		if err != nil {
			return "", -1, fmt.Errorf("container wait: %w", err)
		}
	case status := <-statusCh:
		exitCode = int(status.StatusCode)
	}

	rc, err := o.cli.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", exitCode, fmt.Errorf("container logs: %w", err)
	}
	defer rc.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return "", exitCode, fmt.Errorf("read logs: %w", err)
	}
	return buf.String(), exitCode, nil
}

// buildPlaywrightScript constructs the inline Node.js Playwright verification
// script. Navigates to http://app-under-test:<port>, asserts HTTP 200,
// captures console errors and DOM snapshot, emits a JSON result block.
func buildPlaywrightScript(port string) string {
	return fmt.Sprintf(`
const { chromium } = require('@playwright/test');
(async () => {
  const browser = await chromium.launch({ args: ['--no-sandbox', '--disable-setuid-sandbox'] });
  const page = await browser.newPage();
  const consoleErrors = [];
  page.on('console', msg => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });

  let statusCode = 0;
  let domSnapshot = '';
  let passed = false;

  try {
    const response = await page.goto('http://%s:%s/', { timeout: 20000, waitUntil: 'domcontentloaded' });
    statusCode = response ? response.status() : 0;
    passed = statusCode === 200;
    domSnapshot = (await page.content()).substring(0, 2000);
  } catch (err) {
    consoleErrors.push('Navigation error: ' + err.message);
  }

  await browser.close();
  console.log('E2E_RESULT_START');
  console.log(JSON.stringify({ passed, statusCode, domSnapshot, consoleErrors }));
  console.log('E2E_RESULT_END');
  process.exit(passed ? 0 : 1);
})();
`, appAlias, port)
}

// playwrightJSON mirrors the JSON structure emitted by the Playwright script.
type playwrightJSON struct {
	Passed        bool     `json:"passed"`
	StatusCode    int      `json:"statusCode"`
	DOMSnapshot   string   `json:"domSnapshot"`
	ConsoleErrors []string `json:"consoleErrors"`
}

// parseE2EOutput extracts structured test results from the tester container logs.
func parseE2EOutput(logs string, exitCode int) *E2EResult {
	result := &E2EResult{
		Passed:     exitCode == 0,
		StatusCode: exitCode,
		Logs:       logs,
	}

	const startMarker = "E2E_RESULT_START\n"
	const endMarker   = "\nE2E_RESULT_END"

	startIdx := strings.Index(logs, startMarker)
	endIdx := strings.Index(logs, endMarker)
	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		result.DOMOutput = logs
		return result
	}

	jsonStr := strings.TrimSpace(logs[startIdx+len(startMarker) : endIdx])
	var parsed playwrightJSON
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
		result.Passed = parsed.Passed
		result.StatusCode = parsed.StatusCode
		result.DOMOutput = parsed.DOMSnapshot
		result.ConsoleErrors = parsed.ConsoleErrors
	}
	return result
}
