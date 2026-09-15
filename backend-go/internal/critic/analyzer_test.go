package critic

import (
	"math"
	"testing"
)

type fixture struct {
	name         string
	stdout       string
	stderr       string
	exitCode     int
	expectedQ    float64
	expectedPass []string
	expectedFail []string
	hasError     bool
}

func TestAnalyze(t *testing.T) {
	fixtures := []fixture{
		{
			name:      "perfect_run",
			stdout:    "[STAGE:SETUP]\n[STAGE:ASSERTION:t1]\n[TEST_RESULT:PASS:t1]",
			stderr:    "",
			exitCode:  0,
			expectedQ: 1.0,
			hasError:  false,
		},
		{
			name:      "runtime_panic",
			stdout:    "",
			stderr:    "panic: runtime error: index out of range",
			exitCode:  1,
			expectedQ: 0.0,
			hasError:  true,
		},
		{
			name:      "partial_pass",
			stdout:    "[STAGE:ASSERTION:t1]\n[TEST_RESULT:PASS:t1]\n[STAGE:ASSERTION:t2]\n[TEST_RESULT:FAIL:t2]",
			stderr:    "",
			exitCode:  0,
			expectedQ: 0.5,
			hasError:  false,
		},
		{
			name:      "all_fail",
			stdout:    "[STAGE:ASSERTION:t1]\n[TEST_RESULT:FAIL:t1]",
			stderr:    "",
			exitCode:  1,
			expectedQ: 0.2,
			hasError:  false,
		},
		{
			name:      "no_markers",
			stdout:    "some output without any markers",
			stderr:    "",
			exitCode:  0,
			expectedQ: 0.2,
			hasError:  false,
		},
		{
			name:      "python_traceback",
			stdout:    "",
			stderr:    "Traceback (most recent call last):\n  File \"main.py\"\nTypeError: unsupported",
			exitCode:  1,
			expectedQ: 0.0,
			hasError:  true,
		},
		{
			name:      "java_exception",
			stdout:    "",
			stderr:    "Exception in thread \"main\" java.lang.NullPointerException",
			exitCode:  1,
			expectedQ: 0.0,
			hasError:  true,
		},
		{
			name:      "three_of_four_pass",
			stdout:    "[STAGE:ASSERTION:t1]\n[TEST_RESULT:PASS:t1]\n[STAGE:ASSERTION:t2]\n[TEST_RESULT:PASS:t2]\n[STAGE:ASSERTION:t3]\n[TEST_RESULT:PASS:t3]\n[STAGE:ASSERTION:t4]\n[TEST_RESULT:FAIL:t4]",
			stderr:    "",
			exitCode:  0,
			expectedQ: 0.2 + 0.6*0.75, // 0.65
			hasError:  false,
		},
		{
			name:      "stderr_present_exit0",
			stdout:    "[TEST_RESULT:PASS:t1]",
			stderr:    "warning: deprecated",
			exitCode:  0,
			expectedQ: 1.0,
			hasError:  false,
		},
		{
			name:      "empty_output",
			stdout:    "",
			stderr:    "",
			exitCode:  0,
			expectedQ: 0.2,
			hasError:  false,
		},
	}

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			art := ExecutionArtifact{
				Stdout:   f.stdout,
				Stderr:   f.stderr,
				ExitCode: f.exitCode,
			}
			res := Analyze(art)

			if res.HasRuntimeError != f.hasError {
				t.Errorf("expected hasError=%v, got %v", f.hasError, res.HasRuntimeError)
			}
			if math.Abs(res.QualityScore-f.expectedQ) > 1e-9 {
				t.Errorf("expected Q=%f, got %f", f.expectedQ, res.QualityScore)
			}
		})
	}
}
