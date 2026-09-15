package critic

import (
	"bufio"
	"strings"
	"time"
)

// ExecutionArtifact is the raw output from Piston or the DooD sandbox.
type ExecutionArtifact struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Signal   string
	Latency  time.Duration
}

type FailureCategory string

const (
	FailSyntax         FailureCategory = "Syntax"
	FailLogic          FailureCategory = "Logic"
	FailInfrastructure FailureCategory = "Infrastructure"
	FailNone           FailureCategory = "None"
)

// ParsedResult is the structured output of the log analyzer.
type ParsedResult struct {
	HasRuntimeError  bool
	RuntimeErrorText string   // first matching line from stderr
	SetupLogged      bool
	TestedAssertions []string // extracted test names from [STAGE:ASSERTION:*]
	PassedTests      []string // extracted test names from [TEST_RESULT:PASS:*]
	FailedTests      []string // extracted test names from [TEST_RESULT:FAIL:*]
	QualityScore     float64  // Q_actual ∈ [0.0, 1.0]
	LastValidLogLine string   // last non-empty stdout line before first FAIL or error
	FailureCategory  FailureCategory
}

func Analyze(artifact ExecutionArtifact) ParsedResult {
	res := ParsedResult{
		TestedAssertions: make([]string, 0, 8),
		PassedTests:      make([]string, 0, 8),
		FailedTests:      make([]string, 0, 8),
		FailureCategory:  FailNone,
	}

	// Step 1 — Stderr scan (early exit)
	if artifact.Stderr != "" {
		scanner := bufio.NewScanner(strings.NewReader(artifact.Stderr))
		for scanner.Scan() {
			line := scanner.Text()
			for _, re := range reRuntimeErrors {
				if re.MatchString(line) {
					res.HasRuntimeError = true
					res.RuntimeErrorText = line
					break
				}
			}
			if res.HasRuntimeError {
				break
			}
		}
	}
	
	// Step 2 — Stdout scan (line-by-line using bufio.Scanner)
	scanner := bufio.NewScanner(strings.NewReader(artifact.Stdout))
	
	firstFailSeen := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.Contains(line, MarkerSetup) {
			res.SetupLogged = true
		} else if strings.Contains(line, MarkerAssertion) {
			if m := reAssertionName.FindStringSubmatch(line); m != nil {
				res.TestedAssertions = append(res.TestedAssertions, m[1])
			}
		} else if strings.Contains(line, MarkerPass) {
			if m := rePassName.FindStringSubmatch(line); m != nil {
				res.PassedTests = append(res.PassedTests, m[1])
			}
		} else if strings.Contains(line, MarkerFail) {
			if m := reFailName.FindStringSubmatch(line); m != nil {
				res.FailedTests = append(res.FailedTests, m[1])
			}
			firstFailSeen = true
		}

		if !firstFailSeen && !res.HasRuntimeError && trimmed != "" {
			// Don't record marker lines as the "last valid log line" 
			// if it's just the markers themselves.
			if !strings.HasPrefix(trimmed, "[STAGE:") && !strings.HasPrefix(trimmed, "[TEST_RESULT:") {
				res.LastValidLogLine = trimmed
			}
		}
	}

	// Treat non-zero exit as a runtime error if no other tests passed or failed, 
	// or if we didn't explicitly match a regex in stderr but it failed.
	if artifact.ExitCode != 0 && !res.HasRuntimeError && len(res.FailedTests) == 0 && len(res.PassedTests) == 0 {
		res.HasRuntimeError = true
		res.RuntimeErrorText = "Process exited with non-zero code"
	}

	// Step 3 — Score calculation
	if res.HasRuntimeError {
		res.QualityScore = 0.0
	} else if len(res.FailedTests) > 0 && len(res.PassedTests) == 0 {
		res.QualityScore = 0.2
	} else if len(res.PassedTests) == 0 && len(res.TestedAssertions) == 0 {
		res.QualityScore = 0.2
	} else {
		total := len(res.PassedTests) + len(res.FailedTests)
		if total > 0 {
			res.QualityScore = 0.2 + 0.6*(float64(len(res.PassedTests))/float64(total))
		}
	}

	if res.QualityScore >= 0.799 && artifact.ExitCode == 0 && len(res.PassedTests) > 0 {
		res.QualityScore = 1.0
	}

	// Categorize failures
	if res.QualityScore < 0.8 {
		if artifact.Signal == "SIGKILL" || artifact.Signal == "SIGTERM" || artifact.ExitCode == 137 {
			res.FailureCategory = FailInfrastructure
		} else if strings.Contains(strings.ToLower(res.RuntimeErrorText), "syntax") || strings.Contains(strings.ToLower(res.RuntimeErrorText), "compile") {
			res.FailureCategory = FailSyntax
		} else {
			res.FailureCategory = FailLogic
		}
	}

	return res
}
