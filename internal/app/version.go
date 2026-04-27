package app

import (
	"os/exec"
	"strings"
	"sync"
)

// buildVersion is injected at release build time.
// Default stays "dev" for local development and go run workflows.
var buildVersion = "dev"

var (
	versionOnce  sync.Once
	versionCache string
)

func CurrentVersion() string {
	versionOnce.Do(func() {
		versionCache = resolveCurrentVersion()
	})
	return versionCache
}

func resolveCurrentVersion() string {
	injected := strings.TrimSpace(buildVersion)
	if injected != "" && injected != "dev" {
		return injected
	}
	if gitVersion := strings.TrimSpace(gitDescribe()); gitVersion != "" {
		return gitVersion
	}
	if injected != "" {
		return injected
	}
	return "dev"
}

var gitDescribe = func() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func resetVersionCacheForTest() {
	versionOnce = sync.Once{}
	versionCache = ""
}
