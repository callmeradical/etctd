package model

import "time"

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusInReview   Status = "in_review"
	StatusClosed     Status = "closed"
)

type Type string

const (
	TypeBug     Type = "bug"
	TypeFeature Type = "feature"
	TypeTask    Type = "task"
	TypeChore   Type = "chore"
)

type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
	PriorityP4 Priority = "P4"
)

type Issue struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	Status             Status     `json:"status"`
	Type               Type       `json:"type"`
	Priority           Priority   `json:"priority"`
	Minor              bool       `json:"minor"`
	CreatorSession     string     `json:"creator_session"`
	ImplementerSession string     `json:"implementer_session,omitempty"`
	ReviewerSession    string     `json:"reviewer_session,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
}

type LogType string

const (
	LogTypeProgress LogType = "progress"
	LogTypeBlocker  LogType = "blocker"
	LogTypeDecision LogType = "decision"
	LogTypeResult   LogType = "result"
)

type LogEntry struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issue_id"`
	SessionID string    `json:"session_id"`
	Message   string    `json:"message"`
	Type      LogType   `json:"type"`
	Timestamp time.Time `json:"timestamp"`
}

type Handoff struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issue_id"`
	SessionID string    `json:"session_id"`
	Done      []string  `json:"done,omitempty"`
	Remaining []string  `json:"remaining,omitempty"`
	Decisions []string  `json:"decisions,omitempty"`
	Uncertain []string  `json:"uncertain,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type IssueDependency struct {
	IssueID     string    `json:"issue_id"`
	DependsOnID string    `json:"depends_on_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type Session struct {
	ID                string    `json:"id"`
	Branch            string    `json:"branch"`
	AgentType         string    `json:"agent_type"`
	AgentPID          int       `json:"agent_pid"`
	PreviousSessionID string    `json:"previous_session_id,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	LastActivity      time.Time `json:"last_activity"`
}

func IsValidStatus(s Status) bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusBlocked, StatusInReview, StatusClosed:
		return true
	default:
		return false
	}
}

func IsValidType(t Type) bool {
	switch t {
	case TypeBug, TypeFeature, TypeTask, TypeChore:
		return true
	default:
		return false
	}
}

func IsValidPriority(p Priority) bool {
	switch p {
	case PriorityP0, PriorityP1, PriorityP2, PriorityP3, PriorityP4:
		return true
	default:
		return false
	}
}
