package models

import "time"

type ContextKey string

const (
	UserIDKey    ContextKey = "userID"
	SessionIDKey ContextKey = "sessionID"
	APIKeysKey   ContextKey = "apiKeys"
)

type UserKeys struct {
	OpenAI      string `bson:"openai"`
	Anthropic   string `bson:"anthropic"`
	Google      string `bson:"google"`
	Groq        string `bson:"groq"`
	HuggingFace string `bson:"huggingface"`
}

type User struct {
	Email     string   `bson:"email"`
	Keys      UserKeys `bson:"keys"`
	SLA       int      `bson:"sla"`
	BudgetUSD float64  `bson:"budget_usd"`
}

type NodeState string

const (
	StatePending    NodeState = "PENDING"
	StateReady      NodeState = "READY"
	StateRunning    NodeState = "RUNNING"
	StateEvaluating NodeState = "EVALUATING"
	StateRetrying   NodeState = "RETRYING"
	StateEscalated  NodeState = "ESCALATED"
	StateSucceeded  NodeState = "SUCCEEDED"
	StateFailed     NodeState = "FAILED"
	StateCancelled  NodeState = "CANCELLED"
)

type EventType string

const (
	EventNodeTransition EventType = "NODE_TRANSITION"
	EventDAGUpdate      EventType = "DAG_UPDATE"
	EventError          EventType = "ERROR"
)

type TaskEvent struct {
	EventID   string    `json:"event_id" bson:"_id,omitempty"`
	UserID    string    `json:"user_id" bson:"user_id"`
	SessionID string    `json:"session_id" bson:"session_id"`
	Type      EventType `json:"type" bson:"type"`
	Payload   any       `json:"payload" bson:"payload"`
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`
}

type TaskNode struct {
	ID           string    `json:"id"`
	ParentID     string    `json:"parent_id,omitempty"`
	Task         string    `json:"task"`
	Domain       int       `json:"domain"`
	PlannedTier  string    `json:"planned_tier"`
	ActualTier   string    `json:"actual_tier"`
	Status       NodeState `json:"status"`
	Result       string    `json:"result,omitempty"`
	ThoughtTrace []string  `json:"thought_trace,omitempty"`
	TokensUsed     int       `json:"tokens_used" bson:"tokens_used"`
	CostUSD        float64   `json:"cost_usd" bson:"cost_usd"`
	CreatedAt      time.Time `json:"created_at" bson:"created_at"`
	UserID         string    `json:"user_id" bson:"user_id"`
	SessionID      string    `json:"session_id" bson:"session_id"`
	Version        int       `json:"version" bson:"version"`
	LeaseExpiresAt time.Time `json:"lease_expires_at" bson:"lease_expires_at"`
}

type DAG struct {
	SessionID string               `json:"session_id"`
	UserID    string               `json:"user_id"`
	Status    string               `json:"status,omitempty"`
	Nodes     map[string]*TaskNode `json:"nodes"`
	Edges     [][]string           `json:"edges"`
}

type ClientMessage struct {
	Type    string             `json:"type"`
	Payload string             `json:"payload"`
	Weights map[string]float64 `json:"weights,omitempty"`
}

type RouterRequest struct {
	PromptTokens        int     `json:"prompt_tokens"`
	ComplexityHeuristic float64 `json:"complexity_heuristic"`
	TaskDomainIdx       int     `json:"task_domain_idx"`
	Q0                  int     `json:"q_0"`
	Q1                  int     `json:"q_1"`
	Q2                  int     `json:"q_2"`
	Q3                  int     `json:"q_3"`
	Q4                  int     `json:"q_4"`
	C0                  float64 `json:"c_0"`
	C1                  float64 `json:"c_1"`
	C2                  float64 `json:"c_2"`
	C3                  float64 `json:"c_3"`
	C4                  float64 `json:"c_4"`
	GPUUtil             float64 `json:"gpu_util"`
	WQ                  float64 `json:"w_q"`
	WC                  float64 `json:"w_c"`
	WL                  float64 `json:"w_l"`
}

type RouterResponse struct {
	ChosenAgentID   int     `json:"chosen_agent_id"`
	ExpectedLatency float64 `json:"expected_latency"`
	ExpectedCost    float64 `json:"expected_cost"`
}