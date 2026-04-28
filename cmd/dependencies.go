package cmd

import (
	"os/exec"

	"github.com/teliaz/dot-vault/internal/tui"
)

func collectTUIDependencies() []tui.Dependency {
	return []tui.Dependency{
		externalDependency("git", true, "required for clone and remote-url capture"),
	}
}

func externalDependency(name string, required bool, missingDetail string) tui.Dependency {
	path, err := exec.LookPath(name)
	if err != nil {
		return tui.Dependency{
			Name:      name,
			Required:  required,
			Available: false,
			Detail:    missingDetail,
		}
	}
	return tui.Dependency{
		Name:      name,
		Required:  required,
		Available: true,
		Detail:    path,
	}
}
