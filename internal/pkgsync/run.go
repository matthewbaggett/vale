package pkgsync

import (
	"github.com/vale-cli/vale/v3/internal/core"
	"github.com/vale-cli/vale/v3/internal/system"
)

// Hooks lets a caller watch a sync. Both fields are optional.
type Hooks struct {
	// Plan is called once, before any work, with the number of steps.
	Plan func(total int)

	// Start is called just before each package is installed.
	Start func(name string)

	// Step is called after each package, model or grammar is installed.
	// detail is empty for a package.
	Step func(kind Kind, name, detail string)
}

// Kind says what a Step installed.
type Kind int

// The kinds of thing a sync installs.
const (
	KindPackage Kind = iota
	KindModel
	KindGrammar
)

// Result summarises a sync.
type Result struct {
	StylesPath string
	Packages   int
	Models     int
	Grammars   int

	// Undictionaried names the languages that have a model but no
	// dictionary, so Vale.Spelling skips their files.
	Undictionaried []string
}

// Sync installs everything the configuration names -- packages, language
// models and grammars -- into its StylesPath. It is `vale sync`.
func Sync(flags *core.CLIFlags, hooks Hooks) (*Result, error) {
	cfg, err := core.ReadPipeline(flags, true)
	if err != nil {
		return nil, err
	}

	// NOTE: sync should *only* run for a single config file.
	//
	// We resolve it before touching the `StylesPath` so that a missing config
	// reports itself rather than the empty path it leaves behind.
	rootINI, noRoot := cfg.Root()
	if noRoot != nil {
		return nil, core.NewE100("sync", noRoot)
	} else if err = InitPath(cfg); err != nil {
		return nil, err
	}

	pkgs, err := core.GetPackages(rootINI)
	if err != nil {
		return nil, err
	}
	stylesPath := cfg.StylesPath()

	if hooks.Plan != nil {
		grammars, _ := GrammarsToSync(cfg)
		hooks.Plan(len(pkgs) + len(cfg.Languages()) + len(grammars))
	}

	for idx, pkg := range pkgs {
		name := system.FileNameWithoutExt(pkg)
		if hooks.Start != nil {
			hooks.Start(name)
		}
		if err = ReadPkg(pkg, stylesPath, idx); err != nil {
			return nil, err
		}
		if hooks.Step != nil {
			hooks.Step(KindPackage, name, "")
		}
	}

	stepAs := func(kind Kind) func(name, detail string) {
		return func(name, detail string) {
			if hooks.Step != nil {
				hooks.Step(kind, name, detail)
			}
		}
	}

	// The models for the languages the configuration names, after the
	// packages: a package may be what names a language.
	nModels, err := SyncModels(cfg, stylesPath, len(pkgs), stepAs(KindModel))
	if err != nil {
		return nil, err
	}

	nGrammars, err := SyncGrammars(cfg, stylesPath, len(pkgs)+nModels, stepAs(KindGrammar))
	if err != nil {
		return nil, err
	}

	res := &Result{
		StylesPath: stylesPath,
		Packages:   len(pkgs),
		Models:     nModels,
		Grammars:   nGrammars,
	}

	// A model makes a language's rules work; its spelling needs a
	// dictionary Vale doesn't carry.
	for _, lang := range cfg.Languages() {
		if dic, _ := core.DictionaryFor(cfg, lang); dic == "" {
			res.Undictionaried = append(res.Undictionaried, lang)
		}
	}

	return res, nil
}
