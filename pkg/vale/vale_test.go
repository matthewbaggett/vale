package vale_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vale-cli/vale/v3/pkg/vale"
)

// fixture writes a minimal config and a document with known problems.
func fixture(t *testing.T) (cfg, doc string) {
	t.Helper()
	dir := t.TempDir()
	cfg = filepath.Join(dir, ".vale.ini")
	doc = filepath.Join(dir, "doc.md")
	ini := "StylesPath = styles\nMinAlertLevel = suggestion\n\n[*.md]\nBasedOnStyles = Vale\n"
	if err := os.MkdirAll(filepath.Join(dir, "styles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte("This is is a a teh test.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg, doc
}

func TestLintFilesFindsAlerts(t *testing.T) {
	cfg, doc := fixture(t)
	c, err := vale.New(vale.Options{ConfigPath: cfg, NoGlobal: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Lint(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Total(); got != 3 {
		t.Fatalf("want 3 alerts, got %d: %+v", got, res.Files)
	}
	if !res.HasErrors() {
		t.Fatal("want HasErrors")
	}
}

func TestLintStringMatchesLintFile(t *testing.T) {
	cfg, doc := fixture(t)
	c, err := vale.New(vale.Options{ConfigPath: cfg, NoGlobal: true})
	if err != nil {
		t.Fatal(err)
	}
	fromFile, err := c.Lint(doc)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(doc)
	fromString, err := c.LintStringAs(string(text), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromFile.Files, fromString.Files) {
		t.Fatalf("file and string differ:\n%+v\n%+v", fromFile.Files, fromString.Files)
	}
}

func TestMissingPath(t *testing.T) {
	cfg, _ := fixture(t)
	c, err := vale.New(vale.Options{ConfigPath: cfg, NoGlobal: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Lint("/nonexistent/file.md")
	if e, ok := err.(*vale.Err); !ok || e.Code != "E100" {
		t.Fatalf("want E100 *vale.Err, got %#v", err)
	}
}

func TestCounts(t *testing.T) {
	cfg, doc := fixture(t)
	c, err := vale.New(vale.Options{ConfigPath: cfg, NoGlobal: true, Counts: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Lint(doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["Vale.Repetition"] != 2 || res.Counts["Vale.Spelling"] != 1 {
		t.Fatalf("unexpected counts: %v", res.Counts)
	}
}

// TestParityWithCLI runs the real binary and compares its JSON to the
// facade's. It needs `go build`, so it is skipped with -short.
func TestParityWithCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	cfg, doc := fixture(t)
	bin := filepath.Join(t.TempDir(), "vale")
	build := exec.Command("go", "build", "-o", bin, "github.com/vale-cli/vale/v3/cmd/vale")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "--config="+cfg, "--no-global", "--output=JSON", doc)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run() // exit 1 means alerts were found
	var want map[string][]vale.Alert
	if err := json.Unmarshal(out.Bytes(), &want); err != nil {
		t.Fatalf("parse CLI JSON: %v\n%s", err, out.String())
	}

	c, err := vale.New(vale.Options{ConfigPath: cfg, NoGlobal: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Lint(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, res.Files) {
		t.Fatalf("CLI and facade differ:\nCLI:    %+v\nfacade: %+v", want, res.Files)
	}
}

func TestMetrics(t *testing.T) {
	_, doc := fixture(t)
	m, err := vale.Metrics(doc)
	if err != nil {
		t.Fatal(err)
	}
	if m["words"] == nil {
		t.Fatalf("no word count in %v", m)
	}
}
