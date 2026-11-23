// Package vale is a public facade over Vale's internal packages.
//
// It mirrors what the `vale` binary offers when you shell out to it:
// `vale --output=JSON`, `vale ls-config` and `vale ls-metrics`. The types are
// the same shape as the JSON the CLI prints, so code written against the CLI
// moves over without changes to its data model.
//
//	c, err := vale.New(vale.Options{ConfigPath: ".vale.ini"})
//	if err != nil { ... }
//	res, err := c.LintString("This is is a test.", ".md")
//	for path, alerts := range res.Files { ... }
//
// A Client is safe for sequential use. Do not share one between goroutines.
package vale

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/matthewbaggett/vale/v3/internal/check"
	"github.com/matthewbaggett/vale/v3/internal/core"
	"github.com/matthewbaggett/vale/v3/internal/lint"
	"github.com/matthewbaggett/vale/v3/internal/system"
)

// Severity levels, in order. They match `--minAlertLevel`.
const (
	Suggestion = "suggestion"
	Warning    = "warning"
	Error      = "error"
)

// Options mirror the CLI flags that affect a lint run.
type Options struct {
	// ConfigPath is `--config`. Empty means Vale's normal search for a
	// `.vale.ini`.
	ConfigPath string

	// Sources is `--sources`: a comma-separated list of config files to load.
	Sources string

	// MinAlertLevel is `--minAlertLevel`: "suggestion", "warning" or "error".
	// Empty keeps the configuration's own value.
	MinAlertLevel string

	// Filter is `--filter`: an expression to filter rules by.
	Filter string

	// Glob is `--glob`. It applies to directory input. Empty means "*".
	Glob string

	// IgnoreSyntax is `--ignore-syntax`: lint every file line by line.
	IgnoreSyntax bool

	// NoGlobal is `--no-global`: skip the user-level configuration.
	NoGlobal bool

	// Normalize is `--normalize`: report paths with `/` separators.
	Normalize bool

	// Relative is `--relative`: report relative paths.
	Relative bool

	// Counts is `--counts`: fill Result.Counts with one entry per enabled
	// check, zeros included.
	Counts bool
}

// Action is a possible fix for an Alert.
type Action struct {
	Name   string
	Params []string
}

// Alert is one finding. Field names match the CLI's JSON output.
type Alert struct {
	Action      Action
	Suggestions []string
	Groups      []string `json:",omitempty"`
	Span        []int
	Check       string
	Description string
	Link        string
	Message     string
	Severity    string
	Match       string
	Line        int
}

// Result is what a lint run returns: the CLI's `{path: [alerts]}` map.
type Result struct {
	// Files maps each linted path to its alerts, sorted by position. A file
	// with no alerts is absent, as it is in the CLI's JSON.
	Files map[string][]Alert

	// Counts maps each enabled check to its alert count. Set only when
	// Options.Counts is true.
	Counts map[string]int
}

// HasErrors reports whether any alert has severity "error". The CLI exits 1
// in that case.
func (r *Result) HasErrors() bool {
	for _, alerts := range r.Files {
		for _, a := range alerts {
			if a.Severity == Error {
				return true
			}
		}
	}
	return false
}

// Total is the number of alerts across all files.
func (r *Result) Total() int {
	n := 0
	for _, alerts := range r.Files {
		n += len(alerts)
	}
	return n
}

// Err is a failure of Vale itself, not a finding in a document. Code is
// "E100" for a runtime failure or "E201" for an invalid configuration value.
type Err struct {
	Code    string
	Context string
	Path    string
	Line    int
	Span    int
	Message string
}

func (e *Err) Error() string {
	if e.Path != "" && e.Line > 0 {
		return fmt.Sprintf("%s %s:%d:%d: %s", e.Code, e.Path, e.Line, e.Span, e.Message)
	}
	if e.Context != "" {
		return fmt.Sprintf("%s [%s] %s", e.Code, e.Context, e.Message)
	}
	return fmt.Sprintf("%s %s", e.Code, e.Message)
}

// DocumentError is a failure to lint one document. The run's other results
// are still returned alongside it.
type DocumentError struct {
	Path string
	Err  error
}

func (e *DocumentError) Error() string { return e.Path + ": " + e.Err.Error() }
func (e *DocumentError) Unwrap() error { return e.Err }

// Client lints text with one loaded configuration.
type Client struct {
	opts   Options
	config *core.Config
	linter *lint.Linter
}

// New loads the configuration and builds a linter. It is the equivalent of
// the CLI's start-up, so it fails the way the CLI does on a bad `.vale.ini`.
func New(opts Options) (*Client, error) {
	flags := opts.flags()
	cfg, err := core.ReadPipeline(flags, false)
	if err != nil {
		return nil, wrap(err)
	}
	l, err := lint.NewLinter(cfg)
	if err != nil {
		return nil, wrap(err)
	}
	return &Client{opts: opts, config: cfg, linter: l}, nil
}

func (o Options) flags() *core.CLIFlags {
	glob := o.Glob
	if glob == "" {
		glob = "*"
	}
	return &core.CLIFlags{
		Path:         o.ConfigPath,
		Sources:      o.Sources,
		AlertLevel:   o.MinAlertLevel,
		Filter:       o.Filter,
		Glob:         glob,
		InExt:        ".txt",
		Output:       "JSON",
		Simple:       o.IgnoreSyntax,
		IgnoreGlobal: o.NoGlobal,
		Normalize:    o.Normalize,
		Relative:     o.Relative,
		Counts:       o.Counts,
	}
}

// LintString lints text as a document with the given extension (".md",
// ".html", ".txt" ...). It is the in-memory form of `cat f | vale --ext=.md`.
// An empty ext means ".txt".
func (c *Client) LintString(text, ext string) (*Result, error) {
	return c.LintStringAs(text, "stdin"+normalizeExt(ext))
}

// LintStringAs lints text as the document named name. The name's extension
// picks the format, and the name appears as the key in Result.Files. It is
// the in-memory form of `vale --path=name`.
func (c *Client) LintStringAs(text, name string) (*Result, error) {
	linted, err := c.linter.LintStringAs(text, name)
	return c.result(linted, err)
}

// Lint lints files and directories, as `vale path...` does. Directories are
// walked and filtered by Options.Glob.
func (c *Client) Lint(paths ...string) (*Result, error) {
	if len(paths) == 0 {
		return nil, &Err{Code: "E100", Context: "input", Message: "at least one path expected"}
	}
	for _, p := range paths {
		if !system.FileExists(p) && !system.IsDir(p) {
			return nil, &Err{Code: "E100", Context: "input",
				Message: fmt.Sprintf("argument '%s' does not exist", p)}
		}
	}
	linted, err := c.linter.Lint(paths, c.glob())
	return c.result(linted, err)
}

func (c *Client) glob() string {
	if c.opts.Glob == "" {
		return "*"
	}
	return c.opts.Glob
}

// Config returns the resolved configuration as `vale ls-config` prints it.
func (c *Client) Config() string { return c.config.String() }

// result converts linted files into the public shape. A run that lints some
// documents and fails on others returns both: the Result, and an error
// joining one *DocumentError per failure.
func (c *Client) result(linted []*core.File, err error) (*Result, error) {
	var docErrs lint.DocumentErrors
	if err != nil && !errors.As(err, &docErrs) {
		return nil, wrap(err)
	}

	res := &Result{Files: map[string][]Alert{}}
	for _, f := range linted {
		if f == nil {
			continue
		}
		for _, a := range f.SortedAlerts() {
			res.Files[f.Path] = append(res.Files[f.Path], fromCore(a))
		}
	}
	if c.opts.Counts {
		res.Counts = countAlerts(c.linter.Manager, linted)
	}

	if len(docErrs) > 0 {
		var all []error
		for _, e := range docErrs {
			all = append(all, &DocumentError{Path: errPath(e), Err: wrap(e)})
		}
		return res, errors.Join(all...)
	}
	return res, nil
}

// Metrics returns the counts `vale ls-metrics` prints for one file: words,
// sentences, paragraphs and the elements of its markup. It needs no
// configuration, as in the CLI.
func Metrics(path string) (map[string]interface{}, error) {
	if !system.FileExists(path) || system.IsDir(path) {
		return nil, &Err{Code: "E100", Context: "ls-metrics",
			Message: fmt.Sprintf("'%s' is not a file", path)}
	}
	cfg, err := core.NewConfig(&core.CLIFlags{})
	if err != nil {
		return nil, wrap(err)
	}
	cfg.MinAlertLevel = 0
	cfg.GBaseStyles = []string{"Vale"}
	cfg.Flags.InExt = ".txt"

	l, err := lint.NewLinter(cfg)
	if err != nil {
		return nil, wrap(err)
	}
	linted, err := l.Lint([]string{path}, "*")
	if err != nil {
		return nil, wrap(err)
	}
	if len(linted) == 0 {
		return nil, &Err{Code: "E100", Context: "ls-metrics",
			Message: fmt.Sprintf("'%s' contains no lintable files", path)}
	}
	m, _ := linted[0].ComputeMetrics()
	for k, v := range m {
		if n, ok := v.(float64); ok && n != math.Trunc(n) {
			m[k] = math.Round(n*100) / 100
		}
	}
	return m, nil
}

func fromCore(a core.Alert) Alert {
	return Alert{
		Action:      Action{Name: a.Action.Name, Params: a.Action.Params},
		Suggestions: a.Suggestions,
		Groups:      a.Groups,
		Span:        a.Span,
		Check:       a.Check,
		Description: a.Description,
		Link:        a.Link,
		Message:     a.Message,
		Severity:    a.Severity,
		Match:       a.Match,
		Line:        a.Line,
	}
}

// countAlerts mirrors the CLI's `--counts`: every loaded check starts at
// zero, and runtime names collapse onto the rule that produced them.
func countAlerts(mgr *check.Manager, linted []*core.File) map[string]int {
	counts := map[string]int{}
	for name := range mgr.Rules() {
		counts[name] = 0
	}
	for _, f := range linted {
		if f == nil {
			continue
		}
		for _, a := range f.Alerts {
			name := a.Check
			if _, known := counts[name]; !known {
				name = mgr.RuleForAlert(name)
			}
			counts[name]++
		}
	}
	return counts
}

func wrap(err error) error {
	var ce *core.Error
	if errors.As(err, &ce) {
		return &Err{
			Code: ce.Code, Context: ce.Context, Path: ce.Path,
			Line: ce.Line, Span: ce.Span, Message: ce.Msg,
		}
	}
	return &Err{Code: "E100", Message: err.Error()}
}

func errPath(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) {
		if ce.Path != "" {
			return ce.Path
		}
		return ce.Context
	}
	return ""
}

func normalizeExt(ext string) string {
	if ext == "" {
		return ".txt"
	}
	if !strings.HasPrefix(ext, ".") {
		return "." + ext
	}
	return ext
}
