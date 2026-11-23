package vale

import (
	"github.com/vale-cli/vale/v3/internal/pkgsync"
)

// SyncResult is what `vale sync` reports: what was installed and where.
type SyncResult struct {
	StylesPath string
	Packages   int
	Models     int
	Grammars   int

	// Undictionaried names languages that have a model but no dictionary.
	// Vale.Spelling skips files in those languages.
	Undictionaried []string
}

// Sync installs the packages, language models and grammars that the
// configuration names into its StylesPath. It is `vale sync`.
//
// Only the config-selecting Options apply (ConfigPath, Sources, NoGlobal).
// onStep is optional; it is called after each item is installed. For a
// package detail is empty.
func Sync(opts Options, onStep func(name, detail string)) (*SyncResult, error) {
	hooks := pkgsync.Hooks{}
	if onStep != nil {
		hooks.Step = func(_ pkgsync.Kind, name, detail string) { onStep(name, detail) }
	}
	res, err := pkgsync.Sync(opts.flags(), hooks)
	if err != nil {
		return nil, wrap(err)
	}
	return &SyncResult{
		StylesPath:     res.StylesPath,
		Packages:       res.Packages,
		Models:         res.Models,
		Grammars:       res.Grammars,
		Undictionaried: res.Undictionaried,
	}, nil
}
