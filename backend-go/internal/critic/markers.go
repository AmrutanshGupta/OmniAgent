package critic

import "regexp"

// Structured stdout markers the LLM is instructed to emit.
const (
	MarkerSetup     = "[STAGE:SETUP]"
	MarkerAssertion = "[STAGE:ASSERTION:"  // prefix; full form: [STAGE:ASSERTION:test_name]
	MarkerPass      = "[TEST_RESULT:PASS:" // prefix; full form: [TEST_RESULT:PASS:test_name]
	MarkerFail      = "[TEST_RESULT:FAIL:" // prefix; full form: [TEST_RESULT:FAIL:test_name]
)

// Compiled once at init time — zero-allocation hot path.
var (
	reAssertionName = regexp.MustCompile(`\[STAGE:ASSERTION:([^\]]+)\]`)
	rePassName      = regexp.MustCompile(`\[TEST_RESULT:PASS:([^\]]+)\]`)
	reFailName      = regexp.MustCompile(`\[TEST_RESULT:FAIL:([^\]]+)\]`)

	// stderr error signals — ordered by severity for early-exit scanning.
	reRuntimeErrors = []*regexp.Regexp{
		regexp.MustCompile(`(?i)panic:`),
		regexp.MustCompile(`(?i)segmentation fault`),
		regexp.MustCompile(`(?i)traceback \(most recent call last\)`),
		regexp.MustCompile(`(?i)syntaxerror:`),
		regexp.MustCompile(`(?i)typeerror:`),
		regexp.MustCompile(`(?i)nameerror:`),
		regexp.MustCompile(`(?i)referenceerror:`),
		regexp.MustCompile(`(?i)compileerror:`),
		regexp.MustCompile(`(?i)error\[e`),            // Rust compiler errors
		regexp.MustCompile(`(?i)fatal error:`),        // C/C++
		regexp.MustCompile(`(?i)exception in thread`), // Java
	}
)
