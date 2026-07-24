package formatregistry

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProductionPublicAPIContainsNoV1Writers makes the destructive public API
// cutover durable. Negative tests and the crypto-agility ADR may still name v1
// inputs, but production clients, servers, examples, and user-facing guides
// must not emit or advertise them.
func TestProductionPublicAPIContainsNoV1Writers(t *testing.T) {
	t.Parallel()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate format registry test source")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	scanRoots := []string{
		"cmd/trustdb",
		"internal/adminweb",
		"internal/grpcapi",
		"internal/httpapi",
		"sdk",
		"clients/desktop",
		"clients/web/src",
		"examples",
		"scripts",
		"website/src",
	}
	banned := []string{
		"/" + "v1/",
		"trustdb." + "v1.TrustDB",
		"trustdb/" + "v1/cbor",
	}

	for _, relativeRoot := range scanRoots {
		root := filepath.Join(repositoryRoot, relativeRoot)
		info, err := os.Stat(root)
		if err != nil {
			t.Fatalf("stat scan root %s: %v", relativeRoot, err)
		}
		if !info.IsDir() {
			checkPublicAPICutoverFile(t, repositoryRoot, root, banned)
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "node_modules" || entry.Name() == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			checkPublicAPICutoverFile(t, repositoryRoot, path, banned)
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", relativeRoot, err)
		}
	}
}

func checkPublicAPICutoverFile(t *testing.T, repositoryRoot, path string, banned []string) {
	t.Helper()
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".go", ".js", ".jsx", ".json", ".md", ".ps1", ".sh", ".ts", ".tsx", ".vue", ".yaml", ".yml":
	default:
		return
	}
	if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".test.ts") {
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(content)
	for _, forbidden := range banned {
		if !strings.Contains(text, forbidden) {
			continue
		}
		// TiKV's external Placement Driver health endpoint is not a TrustDB API.
		if forbidden == "/v1/" && strings.Count(text, forbidden) == strings.Count(text, "/pd/api/v1/") {
			continue
		}
		relativePath, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			relativePath = path
		}
		t.Errorf("%s contains retired public API writer %q", filepath.ToSlash(relativePath), forbidden)
	}
}
