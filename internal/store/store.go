package store

import (
	"context"
	"errors"

	"etctd/internal/model"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("write conflict")
)

type SessionIdentity struct {
	Branch    string
	AgentType string
	AgentPID  int
}

type ListIssuesOptions struct {
	Status         model.Status
	Type           model.Type
	Priority       string
	Search         string
	Mine           bool
	IncludeClosed  bool
	ReadyOnly      bool
	ReviewableOnly bool
	Limit          int
}

type Store interface {
	ProjectID() string
	EnsureProject(ctx context.Context) error

	GetOrCreateSession(ctx context.Context, identity SessionIdentity, forceNew bool) (*model.Session, error)

	CreateIssue(ctx context.Context, issue *model.Issue) error
	GetIssue(ctx context.Context, id string) (*model.Issue, int64, error)
	UpdateIssue(ctx context.Context, issue *model.Issue, expectedRevision int64) error
	ListIssues(ctx context.Context, opts ListIssuesOptions, sessionID string) ([]model.Issue, error)

	AppendLog(ctx context.Context, entry *model.LogEntry) error
	ListLogs(ctx context.Context, issueID string, limit int) ([]model.LogEntry, error)

	SaveHandoff(ctx context.Context, handoff *model.Handoff) error
	GetLatestHandoff(ctx context.Context, issueID string) (*model.Handoff, error)

	AddDependency(ctx context.Context, issueID, dependsOnID string) error
	RemoveDependency(ctx context.Context, issueID, dependsOnID string) error
	ListDependencies(ctx context.Context, issueID string) ([]model.IssueDependency, error)
	HasOpenDependencies(ctx context.Context, issueID string) (bool, error)

	Close() error
}
