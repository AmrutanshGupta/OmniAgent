package models

import "time"

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusExecuting  TaskStatus = "executing"
	StatusCompleted  TaskStatus = "completed"
	StatusEscalated  TaskStatus = "escalated"
	StatusFailed     TaskStatus = "failed"
)

// Task is a single node in the execution DAG.
type Task struct {
	ID          string     `json:"id"`
	Domain      string     `json:"domain"` // coding, data, creative
	Description string     `json:"description"`
	DependsOn   []string   `json:"dependsOn"`
	Status      TaskStatus `json:"status"`
	Tier        string     `json:"tier"`          // current model tier assigned by router
	Cascade     []string   `json:"cascade"`        // FrugalGPT escalation ladder e.g. ["flash","sonnet","gpt-4o"]
	CascadeIdx  int        `json:"cascadeIdx"`
	Output      string     `json:"output"`
	TokensUsed  int        `json:"tokensUsed"`
	CostUSD     float64    `json:"costUsd"`
	Attempts    int        `json:"attempts"`
}

// ToTCandidate represents one of the 3 concurrently evaluated architectural approaches.
type ToTCandidate struct {
	Approach       string  `json:"approach"`
	ScalabilityScr float64 `json:"scalabilityScore"`
	CostScore      float64 `json:"costScore"`
	TotalScore     float64 `json:"totalScore"`
	Tasks          []Task  `json:"tasks"`
}

// DAGPlan is the winning ToT plan sent to the frontend visualizer.
type DAGPlan struct {
	ID        string    `json:"id"`
	Prompt    string    `json:"prompt"`
	Winner    string    `json:"winnerApproach"`
	Tasks     []Task    `json:"tasks"`
	CreatedAt time.Time `json:"createdAt"`
}

// SLAWeights come from the frontend "Save Money vs Max Quality" slider.
type SLAWeights struct {
	QualityWeight float64 `json:"qualityWeight"` // 0..1
	CostWeight    float64 `json:"costWeight"`    // 0..1
}

// APIKeyBundle is the decrypted, in-memory-only BYOK set for one request.
type APIKeyBundle struct {
	OpenAI    string `json:"openai,omitempty"`
	Anthropic string `json:"anthropic,omitempty"`
	Google    string `json:"google,omitempty"`
}

type PlanRequest struct {
	Prompt string       `json:"prompt"`
	SLA    SLAWeights   `json:"sla"`
	Keys   APIKeyBundle `json:"keys"`
}

type CostEvent struct {
	TaskID      string  `json:"taskId"`
	Tier        string  `json:"tier"`
	TokensUsed  int     `json:"tokensUsed"`
	CostUSD     float64 `json:"costUsd"`
	SavedUSD    float64 `json:"savedUsd"` // vs always using top-tier model
}
