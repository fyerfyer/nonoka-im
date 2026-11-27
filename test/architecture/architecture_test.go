package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// packageDeps returns the transitive import graph of one package.
func packageDeps(t *testing.T, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func assertNoDeps(t *testing.T, pkg string, forbidden ...string) {
	t.Helper()
	deps := packageDeps(t, pkg)
	for _, dep := range deps {
		for _, prefix := range forbidden {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Errorf("%s must not depend on %s (found %s)", pkg, prefix, dep)
			}
		}
	}
}

func TestServiceDoesNotDependOnInfrastructure(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/service",
		"nonoka-im/internal/data",
		"nonoka-im/internal/msgworker",
		"nonoka-im/internal/filestore",
	)
}

func TestAgentAppDoesNotDependOnAdapters(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/agent/app",
		"nonoka-im/internal/agent/eino",
		"nonoka-im/internal/data",
		"nonoka-im/internal/msgworker",
		"nonoka-im/internal/filestore",
	)
}

func TestPureDomains(t *testing.T) {
	forbidden := []string{
		"gorm.io/gorm",
		"go.mongodb.org/mongo-driver",
		"nonoka-im/internal/agent/eino",
		"nonoka-im/api/im/v1",
		"net/http",
	}
	for _, pkg := range []string{
		"nonoka-im/internal/agent/domain",
		"nonoka-im/internal/im/domain",
		"nonoka-im/internal/bot/domain",
	} {
		assertNoDeps(t, pkg, forbidden...)
	}
}

func TestBotAppDoesNotDependOnAdapters(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/bot/app",
		"nonoka-im/internal/data",
		"nonoka-im/internal/bot/adapters",
		"nonoka-im/internal/bot/delivery",
		"nonoka-im/internal/bot/stream",
		"nonoka-im/internal/service",
		"gorm.io/gorm",
		"go.mongodb.org/mongo-driver",
	)
}

func TestIMAppDoesNotDependOnAdapters(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/im/app",
		"nonoka-im/internal/im/storage",
		"nonoka-im/internal/im/consumer",
		"nonoka-im/internal/im/push",
		"nonoka-im/internal/data",
		"nonoka-im/internal/msgworker",
		"nonoka-im/internal/authz",
	)
}

func TestGatewayDoesNotDependOnInfrastructure(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/gateway",
		"nonoka-im/internal/data",
		"nonoka-im/internal/msgworker",
		"nonoka-im/internal/im/storage",
	)
}

func TestMessagingIsContractsOnly(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/messaging",
		"nonoka-im/internal/data",
		"nonoka-im/internal/msgworker",
		"nonoka-im/internal/im/storage",
	)
}

func TestDataDoesNotDependOnTransportOrAPI(t *testing.T) {
	cmd := exec.Command("go", "list", "./internal/data/...")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list data packages: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		assertNoDeps(t, pkg,
			"nonoka-im/internal/service",
			"nonoka-im/api/im/v1",
			"nonoka-im/internal/msgworker",
		)
	}
}

func TestAgentBootstrapDoesNotDependOnData(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/agent/bootstrap", "nonoka-im/internal/data")
}

func TestModelServiceDoesNotDependOnAdapters(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/llm",
		"nonoka-im/internal/agent/eino",
		"nonoka-im/internal/data",
		"nonoka-im/internal/service",
	)
}

func TestSkillSpecIsPure(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/agent/skillspec",
		"nonoka-im/internal/agent/eino",
		"nonoka-im/internal/data",
		"gorm.io/gorm",
		"go.mongodb.org/mongo-driver",
		"net/http",
	)
}
