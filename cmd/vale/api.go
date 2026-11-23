package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"

	"github.com/vale-cli/vale/v3/internal/core"
	"github.com/vale-cli/vale/v3/internal/pkgsync"
	"github.com/vale-cli/vale/v3/internal/system"
)

func init() {
	pflag.BoolVar(&Flags.Remote, "mode-rev-compat", false,
		"Prioritize remote Vale configurations.")
	pflag.StringVar(&Flags.Built, "built", "", "A post-processed file path.")

	commands["install"] = command{
		Run:     install,
		Summary: "Install a package from a URL.",
		Hidden:  true,
	}
}

func install(args []string, flags *core.CLIFlags) error {
	cfg, err := core.ReadPipeline(flags, false)
	if err != nil {
		return err
	}

	style := filepath.Join(cfg.StylesPath(), args[0])
	if system.IsDir(style) {
		os.RemoveAll(style) // Remove existing version
	}

	err = pkgsync.Fetch(args[1], cfg.StylesPath())
	if err != nil {
		return sendResponse(
			fmt.Sprintf("Failed to install '%s'", args[1]),
			err)
	}

	return sendResponse(fmt.Sprintf(
		"Successfully installed '%s'", args[1]), nil)
}
