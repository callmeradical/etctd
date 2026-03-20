package project

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Info struct {
	Root string
	ID   string
}

func Detect() (Info, error) {
	root, err := gitRoot()
	if err != nil || root == "" {
		root, err = os.Getwd()
		if err != nil {
			return Info{}, err
		}
	}

	root = filepath.Clean(root)
	h := sha1.Sum([]byte(root))
	pid := "prj_" + hex.EncodeToString(h[:6])

	return Info{Root: root, ID: pid}, nil
}

func gitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
