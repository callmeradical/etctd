package etcd

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"etctd/internal/model"
	"etctd/internal/project"
	"etctd/internal/store"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

func TestDependenciesAndReadyFilter(t *testing.T) {
	ctx := context.Background()
	s := newIntegrationStore(t)

	issueA := testIssue("etc-a", "A", model.TypeTask, model.PriorityP2, model.StatusOpen)
	issueB := testIssue("etc-b", "B", model.TypeTask, model.PriorityP1, model.StatusOpen)
	if err := s.CreateIssue(ctx, issueA); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if err := s.CreateIssue(ctx, issueB); err != nil {
		t.Fatalf("create B: %v", err)
	}

	if err := s.AddDependency(ctx, "etc-a", "etc-b"); err != nil {
		t.Fatalf("add dependency: %v", err)
	}

	hasOpen, err := s.HasOpenDependencies(ctx, "etc-a")
	if err != nil {
		t.Fatalf("has open deps: %v", err)
	}
	if !hasOpen {
		t.Fatalf("expected open dependency")
	}

	ready, err := s.ListIssues(ctx, store.ListIssuesOptions{ReadyOnly: true}, "ses-a")
	if err != nil {
		t.Fatalf("list ready: %v", err)
	}
	if got := issueIDs(ready); len(got) != 1 || got[0] != "etc-b" {
		t.Fatalf("expected only etc-b ready, got %v", got)
	}

	b, rev, err := s.GetIssue(ctx, "etc-b")
	if err != nil {
		t.Fatalf("get B: %v", err)
	}
	b.Status = model.StatusClosed
	b.ClosedAt = ptrTime(time.Now().UTC())
	b.UpdatedAt = time.Now().UTC()
	if err := s.UpdateIssue(ctx, b, rev); err != nil {
		t.Fatalf("close B: %v", err)
	}

	hasOpen, err = s.HasOpenDependencies(ctx, "etc-a")
	if err != nil {
		t.Fatalf("has open deps after close: %v", err)
	}
	if hasOpen {
		t.Fatalf("expected no open dependency after close")
	}

	ready, err = s.ListIssues(ctx, store.ListIssuesOptions{ReadyOnly: true}, "ses-a")
	if err != nil {
		t.Fatalf("list ready after close: %v", err)
	}
	ids := issueIDs(ready)
	if len(ids) != 1 || ids[0] != "etc-a" {
		t.Fatalf("expected only etc-a ready after close, got %v", ids)
	}
}

func TestQueryFiltersAndReviewable(t *testing.T) {
	ctx := context.Background()
	s := newIntegrationStore(t)

	if err := s.CreateIssue(ctx, &model.Issue{
		ID:                 "etc-r1",
		Title:              "Review me",
		Status:             model.StatusInReview,
		Type:               model.TypeTask,
		Priority:           model.PriorityP2,
		ImplementerSession: "ses-impl",
		CreatorSession:     "ses-create",
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create review issue: %v", err)
	}

	if err := s.CreateIssue(ctx, &model.Issue{
		ID:                 "etc-r2",
		Title:              "Minor self review",
		Status:             model.StatusInReview,
		Type:               model.TypeTask,
		Priority:           model.PriorityP2,
		Minor:              true,
		ImplementerSession: "ses-impl",
		CreatorSession:     "ses-create",
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create minor review issue: %v", err)
	}

	if err := s.CreateIssue(ctx, &model.Issue{
		ID:             "etc-bug",
		Title:          "Scheduler bug in reconcile loop",
		Description:    "query scheduler bug",
		Status:         model.StatusOpen,
		Type:           model.TypeBug,
		Priority:       model.PriorityP1,
		CreatorSession: "ses-create",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create bug issue: %v", err)
	}

	list, err := s.ListIssues(ctx, store.ListIssuesOptions{ReviewableOnly: true}, "ses-impl")
	if err != nil {
		t.Fatalf("reviewable list (self): %v", err)
	}
	if ids := issueIDs(list); len(ids) != 1 || ids[0] != "etc-r2" {
		t.Fatalf("expected only minor self-review issue, got %v", ids)
	}

	list, err = s.ListIssues(ctx, store.ListIssuesOptions{ReviewableOnly: true}, "ses-other")
	if err != nil {
		t.Fatalf("reviewable list (other): %v", err)
	}
	if ids := issueIDs(list); len(ids) != 2 || !contains(ids, "etc-r1") || !contains(ids, "etc-r2") {
		t.Fatalf("expected etc-r1 and etc-r2 reviewable for other session, got %v", ids)
	}

	list, err = s.ListIssues(ctx, store.ListIssuesOptions{Type: model.TypeBug, Priority: "<=P1", Search: "scheduler"}, "ses-other")
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if ids := issueIDs(list); len(ids) != 1 || ids[0] != "etc-bug" {
		t.Fatalf("expected filtered bug issue, got %v", ids)
	}
}

func TestUpdateIssueConflict(t *testing.T) {
	ctx := context.Background()
	s := newIntegrationStore(t)

	issue := testIssue("etc-cas", "CAS", model.TypeTask, model.PriorityP2, model.StatusOpen)
	if err := s.CreateIssue(ctx, issue); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	loaded, rev, err := s.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}

	loaded.Title = "CAS updated"
	loaded.UpdatedAt = time.Now().UTC()
	if err := s.UpdateIssue(ctx, loaded, rev); err != nil {
		t.Fatalf("first update: %v", err)
	}

	loaded.Title = "stale revision update"
	loaded.UpdatedAt = time.Now().UTC()
	err = s.UpdateIssue(ctx, loaded, rev)
	if err != store.ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func newIntegrationStore(t *testing.T) *Store {
	t.Helper()

	ctx := context.Background()
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.Logger = "zap"
	cfg.LogLevel = "error"

	clientURL, _ := url.Parse("http://127.0.0.1:0")
	peerURL, _ := url.Parse("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start embedded etcd: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		t.Fatal("embedded etcd not ready")
	}

	endpoint := e.Clients[0].Addr().String()
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "http://" + endpoint
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
		Context:     ctx,
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	proj := project.Info{ID: "prj_it_" + generateID("", 3), Root: t.TempDir()}
	store := &Store{
		client:      cli,
		project:     proj,
		projectPath: defaultPrefix + "/projects/" + proj.ID,
	}

	if err := store.EnsureProject(ctx); err != nil {
		t.Fatalf("ensure project: %v", err)
	}

	return store
}

func testIssue(id, title string, typ model.Type, priority model.Priority, status model.Status) *model.Issue {
	now := time.Now().UTC()
	return &model.Issue{
		ID:             id,
		Title:          title,
		Status:         status,
		Type:           typ,
		Priority:       priority,
		CreatorSession: "ses-create",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func issueIDs(issues []model.Issue) []string {
	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	sort.Strings(ids)
	return ids
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
