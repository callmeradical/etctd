package scenarios_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestCLIActionScenarios(t *testing.T) {
	if os.Getenv("ETCTD_SCENARIOS") != "1" {
		t.Skip("set ETCTD_SCENARIOS=1 to run scenario tests")
	}

	endpoint := strings.TrimSpace(os.Getenv("SMITH_ETCD_ENDPOINTS"))
	if endpoint == "" {
		t.Fatal("SMITH_ETCD_ENDPOINTS is required for scenario tests")
	}

	binary := strings.TrimSpace(os.Getenv("ETCTD_BIN"))
	if binary == "" {
		repoRoot := projectRoot(t)
		binary = buildBinary(t, repoRoot)
	}
	workspace := newWorkspace(t)

	implEnv := map[string]string{
		"SMITH_ETCD_ENDPOINTS": endpoint,
		"SMITH_AGENT_TYPE":     "scenario-impl",
	}
	reviewerEnv := map[string]string{
		"SMITH_ETCD_ENDPOINTS": endpoint,
		"SMITH_AGENT_TYPE":     "scenario-reviewer",
	}

	usage := mustRun(t, workspace, binary, implEnv, "usage", "--new-session")
	assertContains(t, usage, "CURRENT SESSION:")

	usageQuiet := mustRun(t, workspace, binary, implEnv, "usage", "-q")
	assertContains(t, usageQuiet, "CURRENT SESSION:")

	initOut := mustRun(t, workspace, binary, implEnv, "init")
	assertContains(t, initOut, "Project initialized in etcd")

	depID := parseCreatedIssueID(t, mustRun(t, workspace, binary, implEnv, "create", "--type", "task", "--priority", "P2", "Dependency implementation"))
	mainID := parseCreatedIssueID(t, mustRun(t, workspace, binary, implEnv, "create", "--type", "feature", "--priority", "P1", "Main workflow implementation"))

	listOut := mustRun(t, workspace, binary, implEnv, "list", "--search", "Main workflow")
	assertContains(t, listOut, mainID)

	showOut := mustRun(t, workspace, binary, implEnv, "show", mainID)
	assertContains(t, showOut, "Status: open")

	depAddOut := mustRun(t, workspace, binary, implEnv, "dep", "add", mainID, depID)
	assertContains(t, depAddOut, "DEPENDENCY ADDED")

	depListOut := mustRun(t, workspace, binary, implEnv, "dep", "list", mainID)
	assertContains(t, depListOut, depID)

	blockedStartOut, blockedStartErr := runCmd(workspace, binary, implEnv, "start", mainID)
	if blockedStartErr == nil {
		t.Fatalf("expected start to fail for blocked issue; output:\n%s", blockedStartOut)
	}
	assertContains(t, blockedStartOut, "open dependencies")

	startDepOut := mustRun(t, workspace, binary, implEnv, "start", depID)
	assertContains(t, startDepOut, "STARTED")

	logOut := mustRun(t, workspace, binary, implEnv, "log", depID, "Dependency code implemented")
	assertContains(t, logOut, "LOGGED")

	handoffOut := mustRun(t, workspace, binary, implEnv, "handoff", depID,
		"--done", "API wiring,tests",
		"--remaining", "docs",
		"--decisions", "kept interfaces small",
		"--uncertain", "naming",
	)
	assertContains(t, handoffOut, "HANDOFF")

	currentOut := mustRun(t, workspace, binary, implEnv, "current")
	assertContains(t, currentOut, depID)

	reviewOut := mustRun(t, workspace, binary, implEnv, "review", depID)
	assertContains(t, reviewOut, "IN REVIEW")

	reviewableOut := mustRun(t, workspace, binary, reviewerEnv, "list", "--reviewable")
	assertContains(t, reviewableOut, depID)

	approveOut := mustRun(t, workspace, binary, reviewerEnv, "approve", depID)
	assertContains(t, approveOut, "APPROVED")

	mustRun(t, workspace, binary, implEnv, "start", mainID)
	mustRun(t, workspace, binary, implEnv, "log", mainID, "Main work in progress")
	mustRun(t, workspace, binary, implEnv, "review", mainID)

	rejectOut := mustRun(t, workspace, binary, reviewerEnv, "reject", mainID, "--reason", "needs more tests")
	assertContains(t, rejectOut, "REJECTED")

	showAfterReject := mustRun(t, workspace, binary, implEnv, "show", mainID)
	assertContains(t, showAfterReject, "Status: in_progress")

	depRemoveOut := mustRun(t, workspace, binary, implEnv, "dep", "remove", mainID, depID)
	assertContains(t, depRemoveOut, "DEPENDENCY REMOVED")

	minorID := parseCreatedIssueID(t, mustRun(t, workspace, binary, implEnv, "create", "--minor", "Minor polish"))
	mustRun(t, workspace, binary, implEnv, "start", minorID)
	mustRun(t, workspace, binary, implEnv, "review", minorID)
	mustRun(t, workspace, binary, reviewerEnv, "approve", minorID)

	readyID := parseCreatedIssueID(t, mustRun(t, workspace, binary, implEnv, "create", "Ready queue issue"))
	readyOut := mustRun(t, workspace, binary, implEnv, "list", "--ready")
	assertContains(t, readyOut, readyID)
}

func mustRun(t *testing.T, dir, binary string, extraEnv map[string]string, args ...string) string {
	t.Helper()
	out, err := runCmd(dir, binary, extraEnv, args...)
	if err != nil {
		t.Fatalf("command failed: %s %s\nerror: %v\noutput:\n%s", binary, strings.Join(args, " "), err, out)
	}
	return out
}

func runCmd(dir, binary string, extraEnv map[string]string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func parseCreatedIssueID(t *testing.T, out string) string {
	t.Helper()
	re := regexp.MustCompile(`CREATED\s+(etc-[a-f0-9]+)\b`)
	m := re.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("failed to parse created issue id from output:\n%s", out)
	}
	return m[1]
}

func assertContains(t *testing.T, out, needle string) {
	t.Helper()
	if !strings.Contains(out, needle) {
		t.Fatalf("expected output to contain %q\noutput:\n%s", needle, out)
	}
}

func buildBinary(t *testing.T, root string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "etctd")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/etctd")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build etctd binary failed: %v\noutput:\n%s", err, string(out))
	}
	return binary
}

func newWorkspace(t *testing.T) string {
	t.Helper()
	ws := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = ws
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init workspace failed: %v\noutput:\n%s", err, string(out))
	}
	return ws
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
