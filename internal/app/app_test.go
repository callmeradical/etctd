package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"etctd/internal/model"
	"etctd/internal/store"
)

type mockStore struct {
	session           *model.Session
	issues            map[string]*model.Issue
	listIssuesResult  []model.Issue
	listIssuesOpts    store.ListIssuesOptions
	listIssuesSession string

	hasOpenDeps map[string]bool

	depAddedIssue string
	depAddedOn    string
}

func (m *mockStore) ProjectID() string { return "prj_test" }
func (m *mockStore) EnsureProject(context.Context) error {
	return nil
}
func (m *mockStore) GetOrCreateSession(context.Context, store.SessionIdentity, bool) (*model.Session, error) {
	if m.session != nil {
		return m.session, nil
	}
	return &model.Session{ID: "ses_test", Branch: "main", AgentType: "terminal", AgentPID: 1, StartedAt: time.Now(), LastActivity: time.Now()}, nil
}
func (m *mockStore) CreateIssue(context.Context, *model.Issue) error { return nil }
func (m *mockStore) GetIssue(_ context.Context, id string) (*model.Issue, int64, error) {
	if m.issues == nil {
		return nil, 0, store.ErrNotFound
	}
	if !strings.HasPrefix(id, "etc-") {
		id = "etc-" + id
	}
	issue, ok := m.issues[id]
	if !ok {
		return nil, 0, store.ErrNotFound
	}
	cpy := *issue
	return &cpy, 1, nil
}
func (m *mockStore) UpdateIssue(_ context.Context, issue *model.Issue, _ int64) error {
	if m.issues == nil {
		m.issues = map[string]*model.Issue{}
	}
	cpy := *issue
	m.issues[issue.ID] = &cpy
	return nil
}
func (m *mockStore) ListIssues(_ context.Context, opts store.ListIssuesOptions, sessionID string) ([]model.Issue, error) {
	m.listIssuesOpts = opts
	m.listIssuesSession = sessionID
	return m.listIssuesResult, nil
}
func (m *mockStore) AppendLog(context.Context, *model.LogEntry) error { return nil }
func (m *mockStore) ListLogs(context.Context, string, int) ([]model.LogEntry, error) {
	return nil, nil
}
func (m *mockStore) SaveHandoff(context.Context, *model.Handoff) error { return nil }
func (m *mockStore) GetLatestHandoff(context.Context, string) (*model.Handoff, error) {
	return nil, store.ErrNotFound
}
func (m *mockStore) AddDependency(_ context.Context, issueID, dependsOnID string) error {
	m.depAddedIssue = issueID
	m.depAddedOn = dependsOnID
	return nil
}
func (m *mockStore) RemoveDependency(context.Context, string, string) error { return nil }
func (m *mockStore) ListDependencies(context.Context, string) ([]model.IssueDependency, error) {
	return nil, nil
}
func (m *mockStore) HasOpenDependencies(_ context.Context, issueID string) (bool, error) {
	if m.hasOpenDeps == nil {
		return false, nil
	}
	if !strings.HasPrefix(issueID, "etc-") {
		issueID = "etc-" + issueID
	}
	return m.hasOpenDeps[issueID], nil
}
func (m *mockStore) Close() error { return nil }

func TestListPassesQueryFilters(t *testing.T) {
	ms := &mockStore{session: &model.Session{ID: "ses_x", Branch: "main", AgentType: "terminal", AgentPID: 1}}
	out := &bytes.Buffer{}
	app := New(ms, out, &bytes.Buffer{})

	err := app.Run(context.Background(), []string{"list", "--type", "bug", "--priority", "<=P2", "--search", "scheduler", "--ready", "--reviewable", "--all", "--mine", "--limit", "7"})
	if err != nil {
		t.Fatalf("run list: %v", err)
	}

	if ms.listIssuesOpts.Type != model.TypeBug {
		t.Fatalf("type filter mismatch: got %q", ms.listIssuesOpts.Type)
	}
	if ms.listIssuesOpts.Priority != "<=P2" {
		t.Fatalf("priority filter mismatch: got %q", ms.listIssuesOpts.Priority)
	}
	if ms.listIssuesOpts.Search != "scheduler" {
		t.Fatalf("search mismatch: got %q", ms.listIssuesOpts.Search)
	}
	if !ms.listIssuesOpts.ReadyOnly || !ms.listIssuesOpts.ReviewableOnly || !ms.listIssuesOpts.IncludeClosed || !ms.listIssuesOpts.Mine {
		t.Fatalf("expected ready/reviewable/all/mine flags to be true: %+v", ms.listIssuesOpts)
	}
	if ms.listIssuesOpts.Limit != 7 {
		t.Fatalf("limit mismatch: got %d", ms.listIssuesOpts.Limit)
	}
}

func TestStartBlocksWhenDependenciesOpen(t *testing.T) {
	ms := &mockStore{
		session: &model.Session{ID: "ses_x", Branch: "main", AgentType: "terminal", AgentPID: 1},
		issues: map[string]*model.Issue{
			"etc-a": {ID: "etc-a", Title: "A", Status: model.StatusOpen, Type: model.TypeTask, Priority: model.PriorityP2},
		},
		hasOpenDeps: map[string]bool{"etc-a": true},
	}

	app := New(ms, &bytes.Buffer{}, &bytes.Buffer{})
	err := app.Run(context.Background(), []string{"start", "a"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "open dependencies") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDepAddCommand(t *testing.T) {
	ms := &mockStore{session: &model.Session{ID: "ses_x", Branch: "main", AgentType: "terminal", AgentPID: 1}}
	out := &bytes.Buffer{}
	app := New(ms, out, &bytes.Buffer{})

	err := app.Run(context.Background(), []string{"dep", "add", "etc-a", "etc-b"})
	if err != nil {
		t.Fatalf("dep add: %v", err)
	}
	if ms.depAddedIssue != "etc-a" || ms.depAddedOn != "etc-b" {
		t.Fatalf("unexpected dependency add call: %s -> %s", ms.depAddedIssue, ms.depAddedOn)
	}
	if !strings.Contains(out.String(), "DEPENDENCY ADDED") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestStartMissingIssue(t *testing.T) {
	ms := &mockStore{session: &model.Session{ID: "ses_x", Branch: "main", AgentType: "terminal", AgentPID: 1}}
	app := New(ms, &bytes.Buffer{}, &bytes.Buffer{})
	err := app.Run(context.Background(), []string{"start", "missing"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
