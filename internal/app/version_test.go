package app

import "testing"

func TestCurrentVersionUsesInjectedBuildVersionWhenProvided(t *testing.T) {
	originalBuildVersion := buildVersion
	originalGitDescribe := gitDescribe
	t.Cleanup(func() {
		buildVersion = originalBuildVersion
		gitDescribe = originalGitDescribe
		resetVersionCacheForTest()
	})

	buildVersion = "v1.2.3"
	gitDescribe = func() string { return "v9.9.9" }
	resetVersionCacheForTest()

	if got := CurrentVersion(); got != "v1.2.3" {
		t.Fatalf("expected injected build version, got %q", got)
	}
}

func TestCurrentVersionFallsBackToGitDescribeWhenBuildVersionIsDev(t *testing.T) {
	originalBuildVersion := buildVersion
	originalGitDescribe := gitDescribe
	t.Cleanup(func() {
		buildVersion = originalBuildVersion
		gitDescribe = originalGitDescribe
		resetVersionCacheForTest()
	})

	buildVersion = "dev"
	gitDescribe = func() string { return "v2.0.1-3-gabc123" }
	resetVersionCacheForTest()

	if got := CurrentVersion(); got != "v2.0.1-3-gabc123" {
		t.Fatalf("expected git describe fallback, got %q", got)
	}
}

func TestCurrentVersionReturnsDevWhenNoInjectedOrGitVersion(t *testing.T) {
	originalBuildVersion := buildVersion
	originalGitDescribe := gitDescribe
	t.Cleanup(func() {
		buildVersion = originalBuildVersion
		gitDescribe = originalGitDescribe
		resetVersionCacheForTest()
	})

	buildVersion = "dev"
	gitDescribe = func() string { return "" }
	resetVersionCacheForTest()

	if got := CurrentVersion(); got != "dev" {
		t.Fatalf("expected dev fallback, got %q", got)
	}
}
