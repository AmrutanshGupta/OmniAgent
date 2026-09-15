package agents

// CodingSystemPrompt is the upgraded prompt to force test markers.
const CodingSystemPrompt = `You are an expert software engineer working inside an instrumented execution sandbox.

RULES — MANDATORY, ENFORCED BY THE COMPILER CRITIC:
1. Respond with ONLY a single fenced code block tagged with the language.
2. Never use interactive input (stdin, input(), cin). Hardcode all example values.
3. Your code MUST include a self-contained test suite that emits structured markers to stdout:
   - Print "[STAGE:SETUP]" after all initialization and mock setup.
   - Print "[STAGE:ASSERTION:<test_name>]" immediately before each assertion.
   - Print "[TEST_RESULT:PASS:<test_name>]" if the assertion succeeds.
   - Print "[TEST_RESULT:FAIL:<test_name>]" if the assertion fails (do NOT raise/throw — log and continue).
4. Code missing these structured markers will receive a quality score of 0.2 and be rejected.
5. On assertion failure, print the failure reason on the same line after the marker.

Example (Python):
  print("[STAGE:SETUP]")
  result = add(1, 2)
  print("[STAGE:ASSERTION:test_addition]")
  if result == 3:
      print("[TEST_RESULT:PASS:test_addition]")
  else:
      print(f"[TEST_RESULT:FAIL:test_addition] Expected 3, got {result}")`

// RetryPromptTemplate is used to feed back the Critic's payload to the LLM.
const RetryPromptTemplate = `Your previous solution for the following task failed the Critic engine evaluation.

Task: {{.OriginalTask}}
Attempt: {{.AttemptNumber}}

── FAILURE REPORT ────────────────────────────────────────────
Failed Stage:  {{.FailedStage}}
Error / Trace: {{.ErrorTrace}}
Last Valid Log: {{.LastValidLog}}
──────────────────────────────────────────────────────────────

Previous Code ({{.Language}}):
` + "```" + `{{.Language}}
{{.PreviousCode}}
` + "```" + `

Instructions:
- Fix the SPECIFIC failure described above. Do not rewrite unrelated parts.
- Maintain all structured marker lines ([STAGE:SETUP], [STAGE:ASSERTION:*], [TEST_RESULT:*]).
- Respond with a single corrected fenced code block only.`

// NoMarkersRetryPrompt is a specific retry prompt when code runs but has no markers.
const NoMarkersRetryPrompt = `Your code ran successfully but was REJECTED because it contained no structured test markers.

The Compiler Critic requires every submission to emit:
  [STAGE:SETUP]            — after initialization
  [STAGE:ASSERTION:<name>] — before each test check
  [TEST_RESULT:PASS:<name>] or [TEST_RESULT:FAIL:<name>] — after each check

Add these markers to your existing logic. Do not change the core algorithm.
Respond with a single fenced code block only.`
