package cmd

import (
	"os/exec"
	"strings"
)

func gitRemoteURL(repoDir string) string {
	output, err := exec.Command("git", "-C", repoDir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
