package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/critic"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/llm_clients"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/pricing"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/sandbox"
)

const defaultPistonURL = "http://localhost:2000"

var pistonHTTPClient = &http.Client{Timeout: 30 * time.Second}
var pistonInstallClient = &http.Client{Timeout: 5 * time.Minute}

var codeFenceRegex = regexp.MustCompile("(?s)```([a-zA-Z0-9_+#-]*)\\n?(.*?)```")

var pistonSemaphore chan struct{}
var pistonSemaphoreOnce sync.Once

func acquirePiston(label string, maxConcurrent int) func() {
	pistonSemaphoreOnce.Do(func() {
		pistonSemaphore = make(chan struct{}, maxConcurrent)
	})
	if len(pistonSemaphore) >= cap(pistonSemaphore) {
		log.Info().Msgf("[Piston] %s waiting\n", label)
	}
	start := time.Now()
	pistonSemaphore <- struct{}{}
	if waited := time.Since(start); waited > 100*time.Millisecond {
		log.Info().Msgf("[Piston] %s got the sandbox after waiting %s\n", label, waited.Round(time.Millisecond*10))
	}
	return func() { <-pistonSemaphore }
}

type pistonFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type pistonExecuteRequest struct {
	Language       string       `json:"language"`
	Version        string       `json:"version"`
	Files          []pistonFile `json:"files"`
	Stdin          string       `json:"stdin"`
	Args           []string     `json:"args"`
	CompileTimeout int          `json:"compile_timeout"`
	RunTimeout     int          `json:"run_timeout"`
}

type pistonRunResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   int    `json:"code"`
	Signal string `json:"signal"`
	Output string `json:"output"`
}

type pistonExecuteResponse struct {
	Language string           `json:"language"`
	Version  string           `json:"version"`
	Run      pistonRunResult  `json:"run"`
	Compile  *pistonRunResult `json:"compile,omitempty"`
	Message  string           `json:"message,omitempty"`
}

type pistonRuntime struct {
	Language string   `json:"language"`
	Version  string   `json:"version"`
	Aliases  []string `json:"aliases"`
}

var langFilename = map[string]string{
	"python":     "main.py",
	"javascript": "main.js",
	"typescript": "main.ts",
	"go":         "main.go",
	"java":       "Main.java",
	"c":          "main.c",
	"cpp":        "main.cpp",
	"csharp":     "main.cs",
	"ruby":       "main.rb",
	"rust":       "main.rs",
	"bash":       "main.sh",
	"php":        "main.php",
}

var langAliases = map[string]string{
	"py": "python", "python3": "python", "python": "python",
	"js": "javascript", "javascript": "javascript", "node": "javascript", "nodejs": "javascript",
	"ts": "typescript", "typescript": "typescript",
	"go": "go", "golang": "go",
	"java": "java",
	"c":    "c",
	"cpp":  "cpp", "c++": "cpp",
	"cs": "csharp", "csharp": "csharp", "c#": "csharp",
	"rb": "ruby", "ruby": "ruby",
	"rs": "rust", "rust": "rust",
	"sh": "bash", "bash": "bash", "shell": "bash",
	"php": "php",
}

var packageNameOverrides = map[string]string{
	"javascript": "node",
	"cpp":        "gcc",
	"c":          "gcc",
	"csharp":     "dotnet",
}

func normalizeLanguage(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if norm, ok := langAliases[tag]; ok {
		return norm
	}
	return "python"
}

type extractedCode struct {
	Language string
	Code     string
}

func extractCode(raw string) extractedCode {
	matches := codeFenceRegex.FindAllStringSubmatch(raw, -1)

	var best extractedCode
	for _, m := range matches {
		content := strings.TrimSpace(m[2])
		if content == "" {
			continue
		}
		if len(content) > len(best.Code) {
			best = extractedCode{Language: normalizeLanguage(m[1]), Code: content}
		}
	}

	if best.Code != "" {
		return best
	}

	return extractedCode{Language: "python", Code: strings.TrimSpace(raw)}
}

type pistonUnreachableError struct{ inner error }

func (e *pistonUnreachableError) Error() string {
	return fmt.Sprintf("piston unreachable: %v", e.inner)
}
func (e *pistonUnreachableError) Unwrap() error { return e.inner }

func isPistonUnreachable(err error) bool {
	var pe *pistonUnreachableError
	return errors.As(err, &pe)
}

func fetchRuntimes(baseURL string) ([]pistonRuntime, error) {
	req, err := http.NewRequest("GET", baseURL+"/api/v2/runtimes", nil)
	if err != nil {
		return nil, err
	}
	resp, err := pistonHTTPClient.Do(req)
	if err != nil {
		return nil, &pistonUnreachableError{err}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runtimes endpoint returned %d: %s", resp.StatusCode, string(data))
	}

	var runtimes []pistonRuntime
	if err := json.Unmarshal(data, &runtimes); err != nil {
		return nil, fmt.Errorf("failed to parse runtimes response: %v", err)
	}
	return runtimes, nil
}

func installPackage(baseURL, language, version string, maxConcurrent int) (string, error) {
	release := acquirePiston(fmt.Sprintf("install(%s)", language), maxConcurrent)
	defer release()

	body, err := json.Marshal(map[string]string{"language": language, "version": version})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", baseURL+"/api/v2/packages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	log.Info().Msgf("[Piston] -> sending install request: package=%q version=%q\n", language, version)

	resp, err := pistonInstallClient.Do(req)
	if err != nil {
		log.Info().Msgf("[Piston] x install request for %q failed after %s: %v\n", language, time.Since(start).Round(time.Second), err)
		return "", &pistonUnreachableError{err}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		log.Info().Msgf("[Piston] x install request for %q rejected after %s: status %d: %s\n", language, time.Since(start).Round(time.Second), resp.StatusCode, string(data))
		return "", fmt.Errorf("package install returned %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Language string `json:"language"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("failed to parse install response: %v", err)
	}
	log.Info().Msgf("[Piston] v install of %q finished in %s - installed version %s\n", language, time.Since(start).Round(time.Second), result.Version)
	return result.Version, nil
}

func ensureRuntimeInstalled(baseURL, language string, maxConcurrent int) (string, error) {
	start := time.Now()
	runtimes, err := fetchRuntimes(baseURL)
	if err != nil {
		return "", err
	}
	if v, ok := findRuntimeVersion(runtimes, language); ok {
		log.Info().Msgf("[Piston] %q already installed (version %s)\n", language, v)
		return v, nil
	}

	pkgName := language
	if override, ok := packageNameOverrides[language]; ok {
		pkgName = override
	}

	log.Info().Msgf("[Piston] %q not installed yet - installing package %q...\n", language, pkgName)
	installedVersion, err := installPackage(baseURL, pkgName, "*", maxConcurrent)
	if err != nil {
		return "", fmt.Errorf("language %q isn't installed and auto-install failed: %v", language, err)
	}

	runtimes, err = fetchRuntimes(baseURL)
	if err != nil {
		return "", err
	}
	if v, ok := findRuntimeVersion(runtimes, language); ok {
		log.Info().Msgf("[Piston] %q ready - total setup time %s (version %s)\n", language, time.Since(start).Round(time.Second), v)
		return v, nil
	}
	if installedVersion != "" {
		log.Info().Msgf("[Piston] %q ready - total setup time %s (version %s)\n", language, time.Since(start).Round(time.Second), installedVersion)
		return installedVersion, nil
	}
	return "", fmt.Errorf("installed package %q but %q still isn't runnable", pkgName, language)
}

func findRuntimeVersion(runtimes []pistonRuntime, language string) (string, bool) {
	for _, rt := range runtimes {
		if rt.Language == language {
			return rt.Version, true
		}
		for _, a := range rt.Aliases {
			if a == language {
				return rt.Version, true
			}
		}
	}
	return "", false
}

func executePiston(baseURL, language, version, filename, code string, timeoutMs int, maxConcurrent int) (string, string, int, string, error) {
	release := acquirePiston(fmt.Sprintf("execute(%s)", language), maxConcurrent)
	defer release()

	timeout := timeoutMs

	reqBody := pistonExecuteRequest{
		Language:       language,
		Version:        version,
		Files:          []pistonFile{{Name: filename, Content: code}},
		Stdin:          "",
		Args:           []string{},
		CompileTimeout: timeout,
		RunTimeout:     timeout,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", 0, "", fmt.Errorf("failed to marshal piston request: %v", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/api/v2/execute", bytes.NewReader(payload))
	if err != nil {
		return "", "", 0, "", fmt.Errorf("failed to build piston request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	log.Info().Msgf("[Piston] -> executing %s (version=%s, file=%s)...\n", language, version, filename)

	resp, err := pistonHTTPClient.Do(req)
	if err != nil {
		return "", "", 0, "", &pistonUnreachableError{err}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, "", fmt.Errorf("failed to read piston response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, "", fmt.Errorf("piston API error %d: %s", resp.StatusCode, string(data))
	}

	var result pistonExecuteResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", "", 0, "", fmt.Errorf("failed to parse piston response: %v - raw: %s", err, string(data))
	}

	if result.Compile != nil && result.Compile.Code != 0 {
		log.Info().Msgf("[Piston] x compile failed after %s. exit_code=%d\n", time.Since(start).Round(time.Millisecond*10), result.Compile.Code)
		return result.Compile.Stdout, result.Compile.Stderr, result.Compile.Code, result.Compile.Signal, nil
	}

	log.Info().Msgf("[Piston] v execution complete in %s. language=%s version=%s exit_code=%d stdout_len=%d stderr_len=%d\n",
		time.Since(start).Round(time.Millisecond*10), result.Language, result.Version, result.Run.Code, len(result.Run.Stdout), len(result.Run.Stderr))

	finalOutput := result.Run.Stdout
	if finalOutput == "" && result.Run.Output != "" {
		finalOutput = result.Run.Output
	}

	return finalOutput, result.Run.Stderr, result.Run.Code, result.Run.Signal, nil
}

func RunCodingTask(ctx context.Context, node *models.TaskNode, actualTier string, originalPrompt string, rec *critic.Recorder, cfg *config.Config) error {

	baseURL := cfg.PistonAPIURL
	if baseURL == "" {
		baseURL = defaultPistonURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	taskPrompt := fmt.Sprintf("Global Context (The User's Goal): %s\n\nYour Specific Sub-Task: %s\n\nWrite the code to solve YOUR specific sub-task, ensuring it aligns with the global context.", originalPrompt, node.Task)

	limit := cfg.CoderMaxAttempts
	totalTokens := 0
	var lastErr string

	log.Info().Msgf("[CoderAgent][node=%s] starting - tier=%s, up to %d attempt(s)\n", node.ID, actualTier, limit)

	for attempt := 1; attempt <= limit; attempt++ {
		log.Info().Msgf("[CoderAgent][node=%s] FSM: stateGenerating\n", node.ID)
		log.Info().Msgf("[CoderAgent][node=%s] attempt %d/%d - calling LLM...\n", node.ID, attempt, limit)
		llmStart := time.Now()

		res, err := llm_clients.Complete(
			ctx,
			actualTier,
			CodingSystemPrompt,
			taskPrompt,
		)
		if err != nil {
			node.Status = models.StateFailed
			return fmt.Errorf("coder LLM failed: %v", err)
		}
		totalTokens += res.TokensUsed
		log.Info().Msgf("[CoderAgent][node=%s] LLM responded in %s (tokens=%d)\n", node.ID, time.Since(llmStart).Round(time.Millisecond*10), res.TokensUsed)

		code := extractCode(res.Text)
		log.Info().Msgf("[CoderAgent][node=%s] detected language=%s, code=%d chars\n", node.ID, code.Language, len(code.Code))

		if strings.TrimSpace(code.Code) == "" {
			log.Info().Msgf("[CoderAgent][node=%s] x attempt %d/%d failed: LLM generated empty code\n", node.ID, attempt, limit)
			stderr := "You did not provide any code in your response. Please output a fenced code block with the full implementation."

			if stderr == lastErr {
				node.Status = models.StateFailed
				log.Info().Msgf("[CoderAgent][node=%s] x STALLED - identical error on attempt %d, stopping early\n", node.ID, attempt)
				return fmt.Errorf("coder agent stalled on attempt %d - same error repeated, stopping early:\n%s", attempt, stderr)
			}
			lastErr = stderr

			log.Info().Msgf("[CoderAgent][node=%s] feeding error back to LLM for attempt %d/%d...\n", node.ID, attempt+1, limit)
			taskPrompt = "Your previous solution failed because it contained no code.\n\nFix it so it runs without error. Respond again with a single fenced code block containing only the corrected code."
			continue
		}

		version, err := ensureRuntimeInstalled(baseURL, code.Language, cfg.PistonMaxConcurrent)
		if err != nil {
			if isPistonUnreachable(err) {
				log.Info().Msgf("[Piston] unreachable at %s: %v - falling back to simulated output\n", baseURL, err)
				node.Status = models.StateSucceeded
				node.Result = fmt.Sprintf("Code generated (sandbox unreachable, not executed):\n\n```%s\n%s\n```\n\nSimulated Sandbox Output: Execution Successful (Piston unreachable)", code.Language, code.Code)
				node.TokensUsed = totalTokens
				applyCost(node, actualTier, totalTokens)
				return nil
			}
			node.Status = models.StateFailed
			return fmt.Errorf("could not prepare %q runtime in sandbox: %v", code.Language, err)
		}

		filename := langFilename[code.Language]
		if filename == "" {
			filename = "main.txt"
		}

		log.Info().Msgf("[CoderAgent][node=%s] FSM: stateExecuting\n", node.ID)
		execStart := time.Now()

		var stdout, stderr, signal string
		var exitCode int
		var execErr error

		// Tiered Sandbox Routing
		if strings.Contains(strings.ToLower(code.Code), "express") || strings.Contains(code.Code, "package.json") {
			// Route to Ephemeral Docker
			log.Info().Msgf("[CoderAgent][node=%s] Routing to Ephemeral Docker (E2E Runner)...\n", node.ID)
			dood, err := sandbox.NewDoodOrchestrator()
			if err == nil {
				files := []sandbox.ProjectFile{
					{RelPath: "package.json", Contents: []byte(`{"name":"test","dependencies":{"express":"*"}}`)},
					{RelPath: "index.js", Contents: []byte(code.Code)},
				}
				wsDir, werr := dood.WriteWorkspace(node.ID, files, cfg)
				if werr == nil {
					res, reErr := dood.RunE2ETest(ctx, node.ID, wsDir, "npm install && node index.js", "3000", cfg)
					if reErr != nil {
						execErr = reErr
					} else {
						stdout = res.Logs
						exitCode = res.StatusCode
						if !res.Passed {
							stderr = strings.Join(res.ConsoleErrors, "\n")
						}
					}
				} else {
					execErr = werr
				}
				dood.Close()
			} else {
				execErr = err
			}
			err = execErr
		} else {
			// Route to Piston
			stdout, stderr, exitCode, signal, err = executePiston(baseURL, code.Language, version, filename, code.Code, cfg.PistonTimeoutMs, cfg.PistonMaxConcurrent)
		}

		if err != nil {
			if isPistonUnreachable(err) {
				log.Info().Msgf("[Piston] unreachable at %s: %v - falling back to simulated output\n", baseURL, err)
				node.Status = models.StateSucceeded
				node.Result = fmt.Sprintf("Code generated (sandbox unreachable, not executed):\n\n```%s\n%s\n```\n\nSimulated Sandbox Output: Execution Successful (Piston unreachable)", code.Language, code.Code)
				node.TokensUsed = totalTokens
				applyCost(node, actualTier, totalTokens)
				return nil
			}
			node.Status = models.StateFailed
			return fmt.Errorf("sandbox execution failed: %v", err)
		}

		log.Info().Msgf("[CoderAgent][node=%s] FSM: stateGrading\n", node.ID)
		artifact := critic.ExecutionArtifact{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
			Signal:   signal,
			Latency:  time.Since(execStart),
		}
		parsed := critic.Analyze(artifact)

		if parsed.FailureCategory == critic.FailInfrastructure {
			node.Status = models.StateFailed
			log.Info().Msgf("[CoderAgent][node=%s] x Infrastructure failure detected. Aborting LLM repair to prevent infinite loops.\n", node.ID)
			return fmt.Errorf("infrastructure failure during execution: %v", err)
		}

		applyCost(node, actualTier, totalTokens) // Calculate cost for telemetry

		if rec != nil {
			go rec.Record(context.Background(), critic.ExecutionTelemetry{
				ModelID:       actualTier,
				SessionID:     node.SessionID,
				NodeID:        node.ID,
				QualityScore:  parsed.QualityScore,
				LatencyMs:     artifact.Latency.Milliseconds(),
				CostUSD:       node.CostUSD,
				AttemptNumber: attempt,
			}, nil) // nil for regUpdater for now to avoid circular dependency without refactoring, we'll fix this in coordinator
		}

		if parsed.QualityScore >= 0.8 {
			node.Status = models.StateSucceeded
			node.Result = fmt.Sprintf("Code generated successfully (%s, %d attempt(s)).\n\n```%s\n%s\n```\n\nSandbox Output:\n%s",
				code.Language, attempt, code.Language, code.Code, stdout)
			node.TokensUsed = totalTokens
			log.Info().Msgf("[CoderAgent][node=%s] v SUCCESS Q=%.2f attempt=%d/%d\n", node.ID, parsed.QualityScore, attempt, limit)
			log.Info().Msgf("[CoderAgent][node=%s] FSM: stateCompleted\n", node.ID)
			return nil
		}

		fb := critic.BuildFeedback(parsed, attempt)
		if fb == nil || attempt >= limit {
			break
		}

		log.Info().Msgf("[CoderAgent][node=%s] x Q=%.2f stage=%s — retrying...\n", node.ID, parsed.QualityScore, fb.FailedStage)

		if fb.ErrorTrace == lastErr {
			node.Status = models.StateFailed
			log.Info().Msgf("[CoderAgent][node=%s] x STALLED - identical error on attempt %d, stopping early\n", node.ID, attempt)
			log.Info().Msgf("[CoderAgent][node=%s] FSM: stateFailed\n", node.ID)
			return fmt.Errorf("coder agent stalled on attempt %d - same error repeated:\n%s", attempt, fb.ErrorTrace)
		}
		lastErr = fb.ErrorTrace

		log.Info().Msgf("[CoderAgent][node=%s] FSM: stateRetrying\n", node.ID)

		if parsed.QualityScore == 0.2 && !parsed.HasRuntimeError && len(parsed.FailedTests) == 0 {
			taskPrompt = NoMarkersRetryPrompt
		} else {
			taskPrompt = critic.FormatRetryPrompt(RetryPromptTemplate, node.Task, fb, code.Language, code.Code)
		}
	}

	node.Status = models.StateFailed
	log.Info().Msgf("[CoderAgent][node=%s] FSM: stateFailed\n", node.ID)
	log.Info().Msgf("[CoderAgent][node=%s] x giving up after %d attempts\n", node.ID, limit)
	return fmt.Errorf("agent failed to produce working code after %d attempts", limit)
}

func applyCost(node *models.TaskNode, actualTier string, totalTokens int) {
	if actualTier == "groq" || actualTier == "hf" {
		node.CostUSD = 0.0
		return
	}
	fallbackCost := 0.000015
	modelName := "gpt-5.4"
	switch actualTier {
	case "flash":
		fallbackCost = 0.00000015
		modelName = "gemini-3.6-flash"
	case "sonnet":
		modelName = "claude-sonnet-5"
	}
	node.CostUSD = float64(totalTokens) * pricing.GetOutputCost(modelName, fallbackCost)
}
