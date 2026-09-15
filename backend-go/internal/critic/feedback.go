package critic

import (
	"bytes"
	"text/template"
)

// FeedbackPayload is injected into the FSM RETRYING state.
type FeedbackPayload struct {
	FailureCategory string `json:"failure_category"`
	FailedStage     string `json:"failed_stage"`
	ErrorTrace      string `json:"error_trace"`
	LastValidLog    string `json:"last_valid_log"`
	AttemptNumber   int    `json:"attempt_number"`
}

// BuildFeedback constructs the structured payload when QualityScore < 0.8
func BuildFeedback(result ParsedResult, attempt int) *FeedbackPayload {
	if result.QualityScore >= 0.8 {
		return nil // no feedback needed
	}

	payload := FeedbackPayload{
		AttemptNumber:   attempt,
		FailureCategory: string(result.FailureCategory),
	}

	if result.HasRuntimeError {
		payload.FailedStage = "RUNTIME_ERROR"
		payload.ErrorTrace = result.RuntimeErrorText
	} else if len(result.FailedTests) > 0 {
		payload.FailedStage = "STAGE:ASSERTION:" + result.FailedTests[0]
		payload.ErrorTrace = "[TEST_RESULT:FAIL:" + result.FailedTests[0] + "]"
	} else {
		payload.FailedStage = "NO_INSTRUMENTATION"
		payload.ErrorTrace = "Code produced no structured markers. Missing [STAGE:SETUP] and [TEST_RESULT:*] lines."
	}

	payload.LastValidLog = result.LastValidLogLine
	return &payload
}

type TemplateData struct {
	OriginalTask    string
	FailureCategory string
	FailedStage     string
	ErrorTrace      string
	LastValidLog    string
	AttemptNumber   int
	Language        string
	PreviousCode    string
}

// FormatRetryPrompt renders the retry prompt.
func FormatRetryPrompt(tpl string, task string, fb *FeedbackPayload, lang, code string) string {
	t := template.Must(template.New("retry").Parse(tpl))
	data := TemplateData{
		OriginalTask:    task,
		FailureCategory: fb.FailureCategory,
		FailedStage:     fb.FailedStage,
		ErrorTrace:      fb.ErrorTrace,
		LastValidLog:    fb.LastValidLog,
		AttemptNumber:   fb.AttemptNumber,
		Language:        lang,
		PreviousCode:    code,
	}
	var buf bytes.Buffer
	err := t.Execute(&buf, data)
	if err != nil {
		return ""
	}
	return buf.String()
}
