package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"etctd/internal/model"
	"etctd/internal/store"
)

type App struct {
	store  store.Store
	out    io.Writer
	errOut io.Writer
}

func New(s store.Store, out io.Writer, errOut io.Writer) *App {
	return &App{store: s, out: out, errOut: errOut}
}

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.printHelp()
		return nil
	}

	if err := a.store.EnsureProject(ctx); err != nil {
		return fmt.Errorf("initialize project metadata: %w", err)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	case "init":
		return a.runInit()
	case "usage":
		return a.runUsage(ctx, rest)
	case "create", "add":
		return a.runCreate(ctx, rest)
	case "list", "ls":
		return a.runList(ctx, rest)
	case "show":
		return a.runShow(ctx, rest)
	case "start":
		return a.runStart(ctx, rest)
	case "review":
		return a.runReview(ctx, rest)
	case "approve":
		return a.runApprove(ctx, rest)
	case "reject":
		return a.runReject(ctx, rest)
	case "log":
		return a.runLog(ctx, rest)
	case "handoff":
		return a.runHandoff(ctx, rest)
	case "dep", "deps", "depends":
		return a.runDeps(ctx, rest)
	case "current":
		return a.runCurrent(ctx)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func (a *App) runInit() error {
	fmt.Fprintf(a.out, "Project initialized in etcd (%s)\n", a.store.ProjectID())
	return nil
}

func (a *App) runUsage(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	quiet := fs.Bool("q", false, "quiet output")
	quietLong := fs.Bool("quiet", false, "quiet output")
	newSession := fs.Bool("new-session", false, "force new session")
	if err := fs.Parse(args); err != nil {
		return err
	}

	sess, err := a.currentSession(ctx, *newSession)
	if err != nil {
		return err
	}

	fmt.Fprintln(a.out, "You have access to `etctd`, an etcd-backed task management CLI.")
	fmt.Fprintln(a.out)
	fmt.Fprintf(a.out, "CURRENT SESSION: %s [%s] on branch: %s\n", sess.ID, sess.AgentType, sess.Branch)

	if *quiet || *quietLong {
		return nil
	}

	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "WORKFLOW:")
	fmt.Fprintln(a.out, "  1. etctd start <id>")
	fmt.Fprintln(a.out, "  2. etctd log <id> \"message\"")
	fmt.Fprintln(a.out, "  3. etctd handoff <id> --done ... --remaining ...")
	fmt.Fprintln(a.out, "  4. etctd review <id>")
	fmt.Fprintln(a.out, "  5. etctd approve <id> (different session)")

	return nil
}

func (a *App) runCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	typeFlag := fs.String("type", string(model.TypeTask), "issue type")
	priorityFlag := fs.String("priority", string(model.PriorityP2), "priority")
	description := fs.String("description", "", "description")
	minor := fs.Bool("minor", false, "allow self-approval")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return errors.New("usage: etctd create [flags] \"title\"")
	}

	title := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if title == "" {
		return errors.New("title is required")
	}

	t := model.Type(*typeFlag)
	p := normalizePriority(*priorityFlag)
	if !model.IsValidType(t) {
		return fmt.Errorf("invalid type %q", *typeFlag)
	}
	if !model.IsValidPriority(p) {
		return fmt.Errorf("invalid priority %q", *priorityFlag)
	}

	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	issue := &model.Issue{
		ID:             generateID("etc-", 4),
		Title:          title,
		Description:    strings.TrimSpace(*description),
		Status:         model.StatusOpen,
		Type:           t,
		Priority:       p,
		Minor:          *minor,
		CreatorSession: sess.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := a.store.CreateIssue(ctx, issue); err != nil {
		return err
	}

	fmt.Fprintf(a.out, "CREATED %s %q\n", issue.ID, issue.Title)
	return nil
}

func (a *App) runList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	statusFlag := fs.String("status", "", "status filter")
	typeFlag := fs.String("type", "", "type filter")
	priority := fs.String("priority", "", "priority filter (P2, <=P2, >=P1)")
	search := fs.String("search", "", "search by id/title/description")
	mine := fs.Bool("mine", false, "only my issues")
	all := fs.Bool("all", false, "include closed issues")
	ready := fs.Bool("ready", false, "only open issues without open dependencies")
	reviewable := fs.Bool("reviewable", false, "only issues you can review")
	limit := fs.Int("limit", 0, "max rows")
	if err := fs.Parse(args); err != nil {
		return err
	}

	status := model.Status(strings.TrimSpace(*statusFlag))
	if status != "" && !model.IsValidStatus(status) {
		return fmt.Errorf("invalid status %q", *statusFlag)
	}

	typ := model.Type(strings.TrimSpace(*typeFlag))
	if typ != "" && !model.IsValidType(typ) {
		return fmt.Errorf("invalid type %q", *typeFlag)
	}

	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	issues, err := a.store.ListIssues(ctx, store.ListIssuesOptions{
		Status:         status,
		Type:           typ,
		Priority:       strings.TrimSpace(*priority),
		Search:         strings.TrimSpace(*search),
		Mine:           *mine,
		IncludeClosed:  *all,
		ReadyOnly:      *ready,
		ReviewableOnly: *reviewable,
		Limit:          *limit,
	}, sess.ID)
	if err != nil {
		return err
	}

	if len(issues) == 0 {
		fmt.Fprintln(a.out, "No issues found")
		return nil
	}

	for _, issue := range issues {
		fmt.Fprintf(a.out, "%s  %-11s %-2s  %s\n", issue.ID, issue.Status, issue.Priority, issue.Title)
	}

	return nil
}

func (a *App) runShow(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: etctd show <id>")
	}

	issue, _, err := a.store.GetIssue(ctx, args[0])
	if err != nil {
		return err
	}

	fmt.Fprintf(a.out, "%s %q\n", issue.ID, issue.Title)
	fmt.Fprintf(a.out, "Status: %s\n", issue.Status)
	fmt.Fprintf(a.out, "Type: %s\n", issue.Type)
	fmt.Fprintf(a.out, "Priority: %s\n", issue.Priority)
	fmt.Fprintf(a.out, "Creator: %s\n", issue.CreatorSession)
	if issue.ImplementerSession != "" {
		fmt.Fprintf(a.out, "Implementer: %s\n", issue.ImplementerSession)
	}
	if issue.ReviewerSession != "" {
		fmt.Fprintf(a.out, "Reviewer: %s\n", issue.ReviewerSession)
	}
	if issue.Description != "" {
		fmt.Fprintf(a.out, "Description: %s\n", issue.Description)
	}
	if issue.ClosedAt != nil {
		fmt.Fprintf(a.out, "Closed: %s\n", issue.ClosedAt.Format(time.RFC3339))
	}

	handoff, hErr := a.store.GetLatestHandoff(ctx, issue.ID)
	if hErr == nil {
		fmt.Fprintf(a.out, "Last handoff: %s\n", handoff.Timestamp.Format(time.RFC3339))
		if len(handoff.Done) > 0 {
			fmt.Fprintf(a.out, "  Done: %s\n", strings.Join(handoff.Done, ", "))
		}
		if len(handoff.Remaining) > 0 {
			fmt.Fprintf(a.out, "  Remaining: %s\n", strings.Join(handoff.Remaining, ", "))
		}
		if len(handoff.Uncertain) > 0 {
			fmt.Fprintf(a.out, "  Uncertain: %s\n", strings.Join(handoff.Uncertain, ", "))
		}
	}

	deps, dErr := a.store.ListDependencies(ctx, issue.ID)
	if dErr == nil && len(deps) > 0 {
		fmt.Fprintln(a.out, "Dependencies:")
		for _, dep := range deps {
			depIssue, _, err := a.store.GetIssue(ctx, dep.DependsOnID)
			if err != nil {
				fmt.Fprintf(a.out, "  - %s (missing)\n", dep.DependsOnID)
				continue
			}
			fmt.Fprintf(a.out, "  - %s [%s] %s\n", depIssue.ID, depIssue.Status, depIssue.Title)
		}
	}

	logs, _ := a.store.ListLogs(ctx, issue.ID, 5)
	if len(logs) > 0 {
		sort.SliceStable(logs, func(i, j int) bool {
			return logs[i].Timestamp.After(logs[j].Timestamp)
		})
		fmt.Fprintln(a.out, "Recent logs:")
		for _, l := range logs {
			fmt.Fprintf(a.out, "  - %s [%s] %s\n", l.Timestamp.Format(time.RFC3339), l.Type, l.Message)
		}
	}

	return nil
}

func (a *App) runStart(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: etctd start <id>")
	}

	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	return a.updateIssue(ctx, args[0], func(issue *model.Issue) error {
		if issue.Status == model.StatusClosed || issue.Status == model.StatusInReview {
			return fmt.Errorf("cannot start issue in status %s", issue.Status)
		}
		hasOpenDeps, err := a.store.HasOpenDependencies(ctx, issue.ID)
		if err != nil {
			return err
		}
		if hasOpenDeps {
			return errors.New("cannot start issue with open dependencies")
		}
		now := time.Now().UTC()
		issue.Status = model.StatusInProgress
		issue.ImplementerSession = sess.ID
		issue.UpdatedAt = now
		issue.ClosedAt = nil
		return nil
	}, "STARTED")
}

func (a *App) runDeps(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: etctd dep <add|remove|list> ...")
	}

	switch args[0] {
	case "add":
		if len(args) != 3 {
			return errors.New("usage: etctd dep add <issue-id> <depends-on-id>")
		}
		if err := a.store.AddDependency(ctx, args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "DEPENDENCY ADDED %s -> %s\n", args[1], args[2])
		return nil
	case "remove", "rm", "delete":
		if len(args) != 3 {
			return errors.New("usage: etctd dep remove <issue-id> <depends-on-id>")
		}
		if err := a.store.RemoveDependency(ctx, args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "DEPENDENCY REMOVED %s -> %s\n", args[1], args[2])
		return nil
	case "list", "ls":
		if len(args) != 2 {
			return errors.New("usage: etctd dep list <issue-id>")
		}
		deps, err := a.store.ListDependencies(ctx, args[1])
		if err != nil {
			return err
		}
		if len(deps) == 0 {
			fmt.Fprintln(a.out, "No dependencies")
			return nil
		}
		for _, dep := range deps {
			fmt.Fprintf(a.out, "%s -> %s\n", dep.IssueID, dep.DependsOnID)
		}
		return nil
	default:
		return fmt.Errorf("unknown dep subcommand %q", args[0])
	}
}

func (a *App) runReview(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: etctd review <id>")
	}

	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	return a.updateIssue(ctx, args[0], func(issue *model.Issue) error {
		if issue.Status != model.StatusInProgress {
			return fmt.Errorf("issue must be in_progress to review (got %s)", issue.Status)
		}
		if issue.ImplementerSession != sess.ID {
			return errors.New("only the implementing session can submit for review")
		}
		issue.Status = model.StatusInReview
		issue.UpdatedAt = time.Now().UTC()
		return nil
	}, "IN REVIEW")
}

func (a *App) runApprove(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: etctd approve <id>")
	}

	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	return a.updateIssue(ctx, args[0], func(issue *model.Issue) error {
		if issue.Status != model.StatusInReview {
			return fmt.Errorf("issue must be in_review to approve (got %s)", issue.Status)
		}
		if !issue.Minor && issue.ImplementerSession == sess.ID {
			return errors.New("you cannot approve an issue you implemented")
		}
		now := time.Now().UTC()
		issue.Status = model.StatusClosed
		issue.ReviewerSession = sess.ID
		issue.UpdatedAt = now
		issue.ClosedAt = &now
		return nil
	}, "APPROVED")
}

func (a *App) runReject(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: etctd reject <id> [--reason text]")
	}

	fs := flag.NewFlagSet("reject", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	reason := fs.String("reason", "", "rejection reason")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	if err := a.updateIssue(ctx, args[0], func(issue *model.Issue) error {
		if issue.Status != model.StatusInReview {
			return fmt.Errorf("issue must be in_review to reject (got %s)", issue.Status)
		}
		if !issue.Minor && issue.ImplementerSession == sess.ID {
			return errors.New("you cannot reject an issue you implemented")
		}
		now := time.Now().UTC()
		issue.Status = model.StatusInProgress
		issue.ReviewerSession = sess.ID
		issue.UpdatedAt = now
		issue.ClosedAt = nil
		return nil
	}, "REJECTED"); err != nil {
		return err
	}

	if strings.TrimSpace(*reason) != "" {
		entry := &model.LogEntry{
			ID:        generateID("log_", 4),
			IssueID:   args[0],
			SessionID: sess.ID,
			Message:   strings.TrimSpace(*reason),
			Type:      model.LogTypeBlocker,
			Timestamp: time.Now().UTC(),
		}
		_ = a.store.AppendLog(ctx, entry)
	}

	return nil
}

func (a *App) runLog(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: etctd log <id> <message>")
	}

	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	issue, _, err := a.store.GetIssue(ctx, args[0])
	if err != nil {
		return err
	}

	if issue.Status == model.StatusClosed {
		return errors.New("cannot add log to closed issue")
	}

	entry := &model.LogEntry{
		ID:        generateID("log_", 4),
		IssueID:   issue.ID,
		SessionID: sess.ID,
		Message:   strings.TrimSpace(strings.Join(args[1:], " ")),
		Type:      model.LogTypeProgress,
		Timestamp: time.Now().UTC(),
	}

	if entry.Message == "" {
		return errors.New("log message cannot be empty")
	}

	if err := a.store.AppendLog(ctx, entry); err != nil {
		return err
	}

	fmt.Fprintf(a.out, "LOGGED %s\n", issue.ID)
	return nil
}

func (a *App) runHandoff(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: etctd handoff <id> [--done a,b] [--remaining c,d]")
	}

	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	done := fs.String("done", "", "completed items")
	remaining := fs.String("remaining", "", "remaining items")
	decisions := fs.String("decisions", "", "decision items")
	uncertain := fs.String("uncertain", "", "uncertain items")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	issue, _, err := a.store.GetIssue(ctx, args[0])
	if err != nil {
		return err
	}

	h := &model.Handoff{
		ID:        generateID("hof_", 4),
		IssueID:   issue.ID,
		SessionID: sess.ID,
		Done:      csvList(*done),
		Remaining: csvList(*remaining),
		Decisions: csvList(*decisions),
		Uncertain: csvList(*uncertain),
		Timestamp: time.Now().UTC(),
	}

	if len(h.Done) == 0 && len(h.Remaining) == 0 && len(h.Decisions) == 0 && len(h.Uncertain) == 0 {
		return errors.New("handoff requires at least one field")
	}

	if err := a.store.SaveHandoff(ctx, h); err != nil {
		return err
	}

	fmt.Fprintf(a.out, "HANDOFF %s saved\n", issue.ID)
	return nil
}

func (a *App) runCurrent(ctx context.Context) error {
	sess, err := a.currentSession(ctx, false)
	if err != nil {
		return err
	}

	issues, err := a.store.ListIssues(ctx, store.ListIssuesOptions{Status: model.StatusInProgress, Mine: true}, sess.ID)
	if err != nil {
		return err
	}

	if len(issues) == 0 {
		fmt.Fprintln(a.out, "No in-progress issues")
		return nil
	}

	for _, issue := range issues {
		fmt.Fprintf(a.out, "%s  %-2s  %s\n", issue.ID, issue.Priority, issue.Title)
	}

	return nil
}

func (a *App) updateIssue(ctx context.Context, id string, update func(issue *model.Issue) error, action string) error {
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		issue, rev, err := a.store.GetIssue(ctx, id)
		if err != nil {
			return err
		}
		if err := update(issue); err != nil {
			return err
		}
		if err := a.store.UpdateIssue(ctx, issue, rev); err != nil {
			if errors.Is(err, store.ErrConflict) {
				continue
			}
			return err
		}
		fmt.Fprintf(a.out, "%s %s\n", action, issue.ID)
		return nil
	}
	return store.ErrConflict
}

func (a *App) currentSession(ctx context.Context, forceNew bool) (*model.Session, error) {
	identity := store.SessionIdentity{
		Branch:    currentBranch(),
		AgentType: currentAgentType(),
		AgentPID:  os.Getppid(),
	}
	return a.store.GetOrCreateSession(ctx, identity, forceNew)
}

func (a *App) printHelp() {
	fmt.Fprintln(a.out, "etctd (etcd-backed, Smith-aligned)")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Usage:")
	fmt.Fprintln(a.out, "  etctd init")
	fmt.Fprintln(a.out, "  etctd usage [-q] [--new-session]")
	fmt.Fprintln(a.out, "  etctd create [--type task] [--priority P2] [--description text] [--minor] \"title\"")
	fmt.Fprintln(a.out, "  etctd list [--status open] [--type bug] [--priority <=P2] [--search term] [--mine] [--ready] [--reviewable] [--all] [--limit N]")
	fmt.Fprintln(a.out, "  etctd show <id>")
	fmt.Fprintln(a.out, "  etctd start <id>")
	fmt.Fprintln(a.out, "  etctd log <id> <message>")
	fmt.Fprintln(a.out, "  etctd handoff <id> --done a,b --remaining c,d")
	fmt.Fprintln(a.out, "  etctd dep add <id> <depends-on-id>")
	fmt.Fprintln(a.out, "  etctd dep list <id>")
	fmt.Fprintln(a.out, "  etctd dep remove <id> <depends-on-id>")
	fmt.Fprintln(a.out, "  etctd review <id>")
	fmt.Fprintln(a.out, "  etctd approve <id>")
	fmt.Fprintln(a.out, "  etctd reject <id> [--reason text]")
	fmt.Fprintln(a.out, "  etctd current")
}

func currentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "default"
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return "default"
	}
	return branch
}

func currentAgentType() string {
	if v := strings.TrimSpace(os.Getenv("SMITH_AGENT_TYPE")); v != "" {
		return v
	}
	if os.Getenv("CLAUDE_CODE_SSE_PORT") != "" {
		return "claude-code"
	}
	if os.Getenv("CURSOR_SESSION_ID") != "" {
		return "cursor"
	}
	if os.Getenv("COPILOT_SESSION_ID") != "" {
		return "copilot"
	}
	return "terminal"
}

func csvList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func generateID(prefix string, bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s%x", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(b)
}

func normalizePriority(raw string) model.Priority {
	v := strings.TrimSpace(strings.ToUpper(raw))
	switch v {
	case "0":
		return model.PriorityP0
	case "1":
		return model.PriorityP1
	case "2":
		return model.PriorityP2
	case "3":
		return model.PriorityP3
	case "4":
		return model.PriorityP4
	default:
		return model.Priority(v)
	}
}
