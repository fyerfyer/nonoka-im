package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
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
		"nonoka-im/internal/gateway",
	)
}

func TestAgentAppDoesNotDependOnAdapters(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/agent/app",
		"nonoka-im/internal/agent/eino",
		"nonoka-im/internal/data",
		"nonoka-im/internal/msgworker",
		"nonoka-im/internal/filestore",
		"nonoka-im/internal/authz",
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

func TestAgentDataDoesNotImportOtherDataSubdomains(t *testing.T) {
	assertNoDeps(t, "nonoka-im/internal/data/agent", "nonoka-im/internal/data/im")
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

// TestSDKPublicAPIHasNoProtoTypes prevents the SDK's public surface from
// leaking generated API packages. Internal implementation may use them.
func TestSDKPublicAPIHasNoProtoTypes(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "pkg", "sdk")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	type parsedFile struct {
		file    *ast.File
		aliases map[string]bool
	}
	var files []parsedFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		aliases := map[string]bool{}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if !strings.HasPrefix(importPath, "nonoka-im/api/") {
				continue
			}
			alias := filepath.Base(importPath)
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			aliases[alias] = true
		}
		files = append(files, parsedFile{file: file, aliases: aliases})
	}

	check := func(aliases map[string]bool, expr ast.Expr) {
		ast.Inspect(expr, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if ok && aliases[ident.Name] {
				t.Errorf("pkg/sdk public API exposes %s.%s", ident.Name, selector.Sel.Name)
			}
			return true
		})
	}
	for _, parsed := range files {
		for _, decl := range parsed.file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if typed.Name.IsExported() {
					check(parsed.aliases, typed.Type)
				}
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch item := spec.(type) {
					case *ast.TypeSpec:
						if item.Name.IsExported() {
							check(parsed.aliases, item.Type)
						}
					case *ast.ValueSpec:
						exported := false
						for _, name := range item.Names {
							exported = exported || name.IsExported()
						}
						if exported && item.Type != nil {
							check(parsed.aliases, item.Type)
						}
					}
				}
			}
		}
	}
}
