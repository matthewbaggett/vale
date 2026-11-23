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

// rulesFixture writes a project whose rule carries one passing and one
// failing in-source case.
func rulesFixture(t *testing.T) (cfg, dir string) {
	t.Helper()
	dir = t.TempDir()
	rule := filepath.Join(dir, "styles", "My", "Foo.yml")
	if err := os.MkdirAll(filepath.Dir(rule), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `extends: existence
message: "Avoid '%s'."
level: warning
tokens:
  - foo
tests:
  - name: fires
    input: "A foo here.\n"
    contains: "My.Foo:Avoid 'foo'."
  - name: clean
    input: "Nothing here.\n"
    want: ""
  - name: wrongly-expects-clean
    input: "Another foo.\n"
    want: ""
`
	if err := os.WriteFile(rule, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg = filepath.Join(dir, ".vale.ini")
	ini := "StylesPath = styles\nMinAlertLevel = suggestion\n\n[*.md]\nBasedOnStyles = My\n"
	if err := os.WriteFile(cfg, []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg, dir
}

func TestTestReportsPassAndFail(t *testing.T) {
	cfg, dir := rulesFixture(t)
	rep, err := vale.Test(
		vale.Options{ConfigPath: cfg, NoGlobal: true},
		vale.TestOptions{Paths: []string{dir}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed != 2 || rep.Failed != 1 {
		t.Fatalf("want 2 passed 1 failed, got %d/%d: %+v", rep.Passed, rep.Failed, rep.Results)
	}
	if rep.OK() {
		t.Fatal("OK() true with a failing case")
	}
}

func TestTestParityWithCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	cfg, dir := rulesFixture(t)
	bin := filepath.Join(t.TempDir(), "vale")
	if out, err := exec.Command("go", "build", "-o", bin,
		"github.com/vale-cli/vale/v3/cmd/vale").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "--config="+cfg, "--no-global", "--output=JSON", "test", dir)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run() // exit 1 means a case failed
	var want vale.TestReport
	if err := json.Unmarshal(out.Bytes(), &want); err != nil {
		t.Fatalf("parse CLI JSON: %v\n%s", err, out.String())
	}

	got, err := vale.Test(
		vale.Options{ConfigPath: cfg, NoGlobal: true},
		vale.TestOptions{Paths: []string{dir}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&want, got) {
		t.Fatalf("CLI and facade differ:\nCLI:    %+v\nfacade: %+v", &want, got)
	}
}

func TestTestNoFiles(t *testing.T) {
	cfg, _ := fixture(t)
	_, err := vale.Test(
		vale.Options{ConfigPath: cfg, NoGlobal: true},
		vale.TestOptions{Paths: []string{t.TempDir()}},
	)
	if e, ok := err.(*vale.Err); !ok || e.Code != "E100" {
		t.Fatalf("want E100 *vale.Err, got %#v", err)
	}
}

func TestSyncLocalPackage(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "local-package", "styles", "TestStyle")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := "extends: existence\nmessage: test\nlevel: warning\ntokens:\n  - x\n"
	if err := os.WriteFile(filepath.Join(pkg, "Rule.yml"), []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(root, ".vale.ini")
	ini := "StylesPath = missing-styles\nPackages = " + filepath.Join(root, "local-package") + "\n"
	if err := os.WriteFile(cfg, []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}

	var steps []string
	res, err := vale.Sync(vale.Options{ConfigPath: cfg, NoGlobal: true},
		func(name, _ string) { steps = append(steps, name) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Packages != 1 || len(steps) != 1 {
		t.Fatalf("want 1 package and 1 step, got %+v steps=%v", res, steps)
	}
	if _, err := os.Stat(filepath.Join(root, "missing-styles", "TestStyle", "Rule.yml")); err != nil {
		t.Fatalf("package not installed: %v", err)
	}
}

func TestSyncMissingConfig(t *testing.T) {
	_, err := vale.Sync(vale.Options{ConfigPath: filepath.Join(t.TempDir(), "nope.ini"), NoGlobal: true}, nil)
	if err == nil {
		t.Fatal("want an error for a missing config")
	}
	if _, ok := err.(*vale.Err); !ok {
		t.Fatalf("want *vale.Err, got %#v", err)
	}
}
