package etcd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"etctd/internal/model"
	"etctd/internal/project"
	"etctd/internal/store"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	defaultPrefix = "/smith/v1/etctd"
	issuePrefix   = "etc-"
)

type Store struct {
	client      *clientv3.Client
	project     project.Info
	projectPath string
}

func Open(ctx context.Context) (*Store, error) {
	proj, err := project.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect project: %w", err)
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints(),
		DialTimeout: etcdDialTimeout(),
		Context:     ctx,
	})
	if err != nil {
		return nil, fmt.Errorf("open etcd: %w", err)
	}

	base := defaultPrefix + "/projects/" + proj.ID
	return &Store{client: cli, project: proj, projectPath: base}, nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

func (s *Store) ProjectID() string {
	return s.project.ID
}

func (s *Store) EnsureProject(ctx context.Context) error {
	metaKey := s.projectMetaKey()
	meta := map[string]string{
		"project_id":   s.project.ID,
		"project_root": s.project.Root,
		"schema":       "v1",
		"updated_at":   time.Now().UTC().Format(time.RFC3339Nano),
	}

	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	_, err = s.client.Put(ctx, metaKey, string(raw))
	return err
}

func (s *Store) GetOrCreateSession(ctx context.Context, identity store.SessionIdentity, forceNew bool) (*model.Session, error) {
	identityKey := s.sessionIdentityKey(identity)

	if !forceNew {
		sidResp, err := s.client.Get(ctx, identityKey)
		if err != nil {
			return nil, err
		}
		if len(sidResp.Kvs) == 1 {
			sid := string(sidResp.Kvs[0].Value)
			sess, rev, err := s.getSessionByID(ctx, sid)
			if err == nil {
				sess.LastActivity = time.Now().UTC()
				raw, mErr := json.Marshal(sess)
				if mErr != nil {
					return nil, mErr
				}

				txn := s.client.Txn(ctx).If(
					clientv3.Compare(clientv3.ModRevision(s.sessionKey(sid)), "=", rev),
				).Then(
					clientv3.OpPut(s.sessionKey(sid), string(raw)),
				)
				if _, eErr := txn.Commit(); eErr != nil {
					return nil, eErr
				}
				return sess, nil
			}
		}
	}

	prevID := ""
	prevResp, err := s.client.Get(ctx, identityKey)
	if err != nil {
		return nil, err
	}
	if len(prevResp.Kvs) == 1 {
		prevID = string(prevResp.Kvs[0].Value)
	}

	now := time.Now().UTC()
	session := &model.Session{
		ID:                generateID("ses_", 4),
		Branch:            identity.Branch,
		AgentType:         identity.AgentType,
		AgentPID:          identity.AgentPID,
		PreviousSessionID: prevID,
		StartedAt:         now,
		LastActivity:      now,
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.Version(s.sessionKey(session.ID)), "=", 0),
	).Then(
		clientv3.OpPut(s.sessionKey(session.ID), string(raw)),
		clientv3.OpPut(identityKey, session.ID),
	)
	txnResp, err := txn.Commit()
	if err != nil {
		return nil, err
	}
	if !txnResp.Succeeded {
		return s.GetOrCreateSession(ctx, identity, forceNew)
	}

	return session, nil
}

func (s *Store) CreateIssue(ctx context.Context, issue *model.Issue) error {
	key := s.issueKey(issue.ID)
	raw, err := json.Marshal(issue)
	if err != nil {
		return err
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.Version(key), "=", 0),
	).Then(
		clientv3.OpPut(key, string(raw)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return err
	}
	if !resp.Succeeded {
		return store.ErrConflict
	}
	return nil
}

func (s *Store) GetIssue(ctx context.Context, id string) (*model.Issue, int64, error) {
	id = normalizeIssueID(id)
	resp, err := s.client.Get(ctx, s.issueKey(id))
	if err != nil {
		return nil, 0, err
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, store.ErrNotFound
	}

	var issue model.Issue
	if err := json.Unmarshal(resp.Kvs[0].Value, &issue); err != nil {
		return nil, 0, err
	}
	return &issue, resp.Kvs[0].ModRevision, nil
}

func (s *Store) UpdateIssue(ctx context.Context, issue *model.Issue, expectedRevision int64) error {
	raw, err := json.Marshal(issue)
	if err != nil {
		return err
	}

	key := s.issueKey(issue.ID)
	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(key), "=", expectedRevision),
	).Then(
		clientv3.OpPut(key, string(raw)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return err
	}
	if !resp.Succeeded {
		return store.ErrConflict
	}
	return nil
}

func (s *Store) ListIssues(ctx context.Context, opts store.ListIssuesOptions, sessionID string) ([]model.Issue, error) {
	resp, err := s.client.Get(ctx, s.issuePrefix(), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	depsByIssue, err := s.buildDependencyMap(ctx)
	if err != nil {
		return nil, err
	}

	issues := make([]model.Issue, 0, len(resp.Kvs))
	issueByID := make(map[string]model.Issue, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var issue model.Issue
		if err := json.Unmarshal(kv.Value, &issue); err != nil {
			continue
		}
		issueByID[issue.ID] = issue
	}

	for _, issue := range issueByID {
		if opts.Status != "" && issue.Status != opts.Status {
			continue
		}
		if opts.Type != "" && issue.Type != opts.Type {
			continue
		}
		if !opts.IncludeClosed && issue.Status == model.StatusClosed {
			continue
		}
		if opts.Priority != "" && !matchesPriorityFilter(issue.Priority, opts.Priority) {
			continue
		}
		if opts.Search != "" {
			needle := strings.ToLower(opts.Search)
			haystack := strings.ToLower(issue.ID + "\n" + issue.Title + "\n" + issue.Description)
			if !strings.Contains(haystack, needle) {
				continue
			}
		}
		if opts.Mine && issue.ImplementerSession != sessionID {
			continue
		}
		if opts.ReviewableOnly {
			if issue.Status != model.StatusInReview {
				continue
			}
			if !issue.Minor && issue.ImplementerSession == sessionID {
				continue
			}
		}
		if opts.ReadyOnly {
			if issue.Status != model.StatusOpen {
				continue
			}
			if hasOpenDependenciesInMap(issue.ID, depsByIssue, issueByID) {
				continue
			}
		}
		issues = append(issues, issue)
	}

	sort.SliceStable(issues, func(i, j int) bool {
		pi := priorityWeight(issues[i].Priority)
		pj := priorityWeight(issues[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return issues[i].UpdatedAt.After(issues[j].UpdatedAt)
	})

	if opts.Limit > 0 && len(issues) > opts.Limit {
		issues = issues[:opts.Limit]
	}

	return issues, nil
}

func (s *Store) AddDependency(ctx context.Context, issueID, dependsOnID string) error {
	issueID = normalizeIssueID(issueID)
	dependsOnID = normalizeIssueID(dependsOnID)

	if issueID == dependsOnID {
		return fmt.Errorf("issue cannot depend on itself")
	}

	if _, _, err := s.GetIssue(ctx, issueID); err != nil {
		return err
	}
	if _, _, err := s.GetIssue(ctx, dependsOnID); err != nil {
		return err
	}

	dep := model.IssueDependency{
		IssueID:     issueID,
		DependsOnID: dependsOnID,
		CreatedAt:   time.Now().UTC(),
	}
	raw, err := json.Marshal(dep)
	if err != nil {
		return err
	}

	key := s.dependencyKey(issueID, dependsOnID)
	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.Version(key), "=", 0),
	).Then(
		clientv3.OpPut(key, string(raw)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return err
	}
	if !resp.Succeeded {
		return store.ErrConflict
	}

	return nil
}

func (s *Store) RemoveDependency(ctx context.Context, issueID, dependsOnID string) error {
	_, err := s.client.Delete(ctx, s.dependencyKey(issueID, dependsOnID))
	return err
}

func (s *Store) ListDependencies(ctx context.Context, issueID string) ([]model.IssueDependency, error) {
	resp, err := s.client.Get(ctx, s.dependencyPrefix(issueID), clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	if err != nil {
		return nil, err
	}

	deps := make([]model.IssueDependency, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var dep model.IssueDependency
		if err := json.Unmarshal(kv.Value, &dep); err != nil {
			continue
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

func (s *Store) HasOpenDependencies(ctx context.Context, issueID string) (bool, error) {
	deps, err := s.ListDependencies(ctx, issueID)
	if err != nil {
		return false, err
	}
	for _, dep := range deps {
		issue, _, err := s.GetIssue(ctx, dep.DependsOnID)
		if err != nil {
			if err == store.ErrNotFound {
				return true, nil
			}
			return false, err
		}
		if issue.Status != model.StatusClosed {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) AppendLog(ctx context.Context, entry *model.LogEntry) error {
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = s.client.Put(ctx, s.logKey(entry.IssueID, entry.Timestamp, entry.ID), string(raw))
	return err
}

func (s *Store) ListLogs(ctx context.Context, issueID string, limit int) ([]model.LogEntry, error) {
	opts := []clientv3.OpOption{clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend)}
	if limit > 0 {
		opts = append(opts, clientv3.WithLimit(int64(limit)))
	}

	resp, err := s.client.Get(ctx, s.logPrefix(issueID), opts...)
	if err != nil {
		return nil, err
	}

	entries := make([]model.LogEntry, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var e model.LogEntry
		if err := json.Unmarshal(kv.Value, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	return entries, nil
}

func (s *Store) SaveHandoff(ctx context.Context, handoff *model.Handoff) error {
	raw, err := json.Marshal(handoff)
	if err != nil {
		return err
	}
	_, err = s.client.Put(ctx, s.handoffKey(handoff.IssueID, handoff.Timestamp, handoff.ID), string(raw))
	return err
}

func (s *Store) GetLatestHandoff(ctx context.Context, issueID string) (*model.Handoff, error) {
	resp, err := s.client.Get(ctx, s.handoffPrefix(issueID), clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortDescend), clientv3.WithLimit(1))
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, store.ErrNotFound
	}
	var h model.Handoff
	if err := json.Unmarshal(resp.Kvs[0].Value, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (s *Store) getSessionByID(ctx context.Context, id string) (*model.Session, int64, error) {
	resp, err := s.client.Get(ctx, s.sessionKey(id))
	if err != nil {
		return nil, 0, err
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, store.ErrNotFound
	}
	var sess model.Session
	if err := json.Unmarshal(resp.Kvs[0].Value, &sess); err != nil {
		return nil, 0, err
	}
	return &sess, resp.Kvs[0].ModRevision, nil
}

func normalizeIssueID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	if strings.HasPrefix(id, issuePrefix) {
		return id
	}
	return issuePrefix + id
}

func (s *Store) projectMetaKey() string {
	return s.projectPath + "/meta"
}

func (s *Store) issuePrefix() string {
	return s.projectPath + "/issues/"
}

func (s *Store) issueKey(id string) string {
	return s.issuePrefix() + normalizeIssueID(id)
}

func (s *Store) logPrefix(issueID string) string {
	return s.projectPath + "/logs/" + normalizeIssueID(issueID) + "/"
}

func (s *Store) logKey(issueID string, ts time.Time, id string) string {
	return s.logPrefix(issueID) + tsKey(ts) + "_" + id
}

func (s *Store) handoffPrefix(issueID string) string {
	return s.projectPath + "/handoffs/" + normalizeIssueID(issueID) + "/"
}

func (s *Store) handoffKey(issueID string, ts time.Time, id string) string {
	return s.handoffPrefix(issueID) + tsKey(ts) + "_" + id
}

func (s *Store) dependencyRootPrefix() string {
	return s.projectPath + "/dependencies/"
}

func (s *Store) dependencyPrefix(issueID string) string {
	return s.dependencyRootPrefix() + normalizeIssueID(issueID) + "/"
}

func (s *Store) dependencyKey(issueID, dependsOnID string) string {
	return s.dependencyPrefix(issueID) + normalizeIssueID(dependsOnID)
}

func (s *Store) sessionPrefix() string {
	return s.projectPath + "/sessions/"
}

func (s *Store) sessionKey(id string) string {
	return s.sessionPrefix() + id
}

func (s *Store) sessionIdentityKey(identity store.SessionIdentity) string {
	tokens := []string{
		url.PathEscape(identity.Branch),
		url.PathEscape(identity.AgentType),
		strconv.Itoa(identity.AgentPID),
	}
	return s.projectPath + "/session-identities/" + strings.Join(tokens, "|")
}

func tsKey(t time.Time) string {
	return fmt.Sprintf("%020d", t.UTC().UnixNano())
}

func generateID(prefix string, bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return prefix + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return prefix + hex.EncodeToString(b)
}

func priorityWeight(p model.Priority) int {
	switch p {
	case model.PriorityP0:
		return 0
	case model.PriorityP1:
		return 1
	case model.PriorityP2:
		return 2
	case model.PriorityP3:
		return 3
	case model.PriorityP4:
		return 4
	default:
		return 9
	}
}

func matchesPriorityFilter(priority model.Priority, filter string) bool {
	f := strings.TrimSpace(strings.ToUpper(filter))
	if f == "" {
		return true
	}

	if strings.HasPrefix(f, "<=") {
		want := strings.TrimSpace(strings.TrimPrefix(f, "<="))
		return priorityWeight(priority) <= priorityWeight(model.Priority(want))
	}
	if strings.HasPrefix(f, ">=") {
		want := strings.TrimSpace(strings.TrimPrefix(f, ">="))
		return priorityWeight(priority) >= priorityWeight(model.Priority(want))
	}

	return strings.EqualFold(string(priority), f)
}

func (s *Store) buildDependencyMap(ctx context.Context) (map[string][]string, error) {
	resp, err := s.client.Get(ctx, s.dependencyRootPrefix(), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	deps := make(map[string][]string)
	for _, kv := range resp.Kvs {
		var dep model.IssueDependency
		if err := json.Unmarshal(kv.Value, &dep); err != nil {
			continue
		}
		deps[dep.IssueID] = append(deps[dep.IssueID], dep.DependsOnID)
	}
	return deps, nil
}

func hasOpenDependenciesInMap(issueID string, depsByIssue map[string][]string, issueByID map[string]model.Issue) bool {
	deps := depsByIssue[issueID]
	for _, depID := range deps {
		depIssue, ok := issueByID[depID]
		if !ok {
			return true
		}
		if depIssue.Status != model.StatusClosed {
			return true
		}
	}
	return false
}

func etcdEndpoints() []string {
	raw := strings.TrimSpace(os.Getenv("SMITH_ETCD_ENDPOINTS"))
	if raw == "" {
		return []string{"http://127.0.0.1:2379"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"http://127.0.0.1:2379"}
	}
	return out
}

func etcdDialTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("SMITH_ETCD_DIAL_TIMEOUT"))
	if raw == "" {
		return 5 * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 5 * time.Second
	}
	return d
}
