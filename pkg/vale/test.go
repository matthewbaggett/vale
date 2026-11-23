package vale

import (
	"errors"
	"fmt"

	"github.com/matthewbaggett/vale/v3/internal/pkgsync"
	"github.com/matthewbaggett/vale/v3/internal/testsuite"
)

// TestResult is one case's outcome. Field names match `vale test
// --output=JSON`.
type TestResult struct {
	File   string `json:"file"`
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
	// Error is why the case could not run at all.
	Error string `json:"error,omitempty"`
	Got   string `json:"got,omitempty"`
	Want  string `json:"want,omitempty"`
	// Tags is the input as tagged, for a failing sequence case.
	Tags string `json:"tags,omitempty"`
}

// TestReport is the outcome of a `vale test` run, in the CLI's JSON shape.
type TestReport struct {
	Passed  int          `json:"passed"`
	Failed  int          `json:"failed"`
	Results []TestResult `json:"results"`

	// Uncovered names the rules no case made fire. Set only when coverage
	// was asked for.
	Uncovered []string `json:"uncovered,omitempty"`
}

// OK reports whether every case passed and, if coverage was asked for,
// every rule fired. The CLI exits 1 when this is false.
func (r *TestReport) OK() bool { return r.Failed == 0 && len(r.Uncovered) == 0 }

// TestOptions are the arguments of `vale test`.
type TestOptions struct {
	// Paths are the test files or directories. Empty means the current
	// directory.
	Paths []string

	// Coverage is `--coverage`: report rules that no case made fire.
	Coverage bool
}

// Test runs the test cases kept beside a configuration's rules. It is
// `vale test`.
//
// A case that fails its assertion is reported in the TestReport, not as an
// error. A case that cannot run at all is also in the report, but Test then
// returns the report together with an E100 *Err, as the CLI exits 2 there.
func Test(opts Options, t TestOptions) (*TestReport, error) {
	paths := t.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}

	files, err := testsuite.Find(paths)
	if err != nil {
		return nil, wrap(testErr(err))
	} else if len(files) == 0 {
		return nil, wrap(testErr(errors.New("no test files found")))
	}

	flags := opts.flags()
	flags.Coverage = t.Coverage

	runner := testsuite.NewRunner(flags)
	runner.Install = func(pkgs []string, styles string) error {
		for idx, pkg := range pkgs {
			if err := pkgsync.ReadPkg(pkg, styles, idx); err != nil {
				return err
			}
		}
		return nil
	}
	defer runner.Close()

	var results []testsuite.Result
	for _, path := range files {
		cases, lErr := testsuite.Load(path)
		if lErr != nil {
			return nil, wrap(testErr(lErr))
		}
		for _, c := range cases {
			results = append(results, runner.Run(c))
		}
	}
	if len(results) == 0 {
		return nil, wrap(testErr(errors.New("no test cases found")))
	}

	report := &TestReport{Results: make([]TestResult, 0, len(results))}
	broken := 0
	for _, r := range results {
		if r.Failed() {
			report.Failed++
		} else {
			report.Passed++
		}
		out := TestResult{
			File:   r.Case.Path,
			Name:   r.Case.Name,
			Passed: !r.Failed(),
			Reason: r.Reason,
			Got:    r.Got,
			Tags:   r.Tags,
		}
		if r.Case.Want != nil {
			out.Want = *r.Case.Want
		}
		if r.Err != nil {
			out.Error = r.Err.Error()
			broken++
		}
		report.Results = append(report.Results, out)
	}

	if t.Coverage {
		rules, rErr := runner.Rules(paths)
		if rErr != nil {
			return nil, wrap(testErr(rErr))
		}
		report.Uncovered = testsuite.Uncovered(rules, results)
	}

	if broken > 0 {
		return report, &Err{Code: "E100", Context: "test",
			Message: fmt.Sprintf("%d case(s) could not run", broken)}
	}
	return report, nil
}

func testErr(err error) error {
	return &Err{Code: "E100", Context: "test", Message: err.Error()}
}
