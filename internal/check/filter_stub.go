//go:build vale_noexpr

package check

import "github.com/matthewbaggett/vale/v3/internal/core"

// runFilter reports that this build cannot evaluate a `--filter`.
func runFilter(_ string, _ FilterEnv) ([]any, error) { return nil, core.ErrNoExpr }
