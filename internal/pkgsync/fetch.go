package pkgsync

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/matthewbaggett/vale/v3/internal/system"
)

// Style represents an externally-hosted style.
type Style struct {
	// User-provided fields.
	Author      string `json:"author"`
	Description string `json:"description"`
	Deps        string `json:"deps"`
	Feed        string `json:"feed"`
	Homepage    string `json:"homepage"`
	Name        string `json:"name"`
	URL         string `json:"url"`

	// Generated fields.
	HasUpdate bool `json:"has_update"`
	InLibrary bool `json:"in_library"`
	Installed bool `json:"installed"`
	Addon     bool `json:"addon"`
}

// Meta represents an installed style's meta data.
type Meta struct {
	Author      string   `json:"author"`
	Coverage    float64  `json:"coverage"`
	Description string   `json:"description"`
	Email       string   `json:"email"`
	Feed        string   `json:"feed"`
	Lang        string   `json:"lang"`
	License     string   `json:"license"`
	Name        string   `json:"name"`
	Sources     []string `json:"sources"`
	URL         string   `json:"url"`
	Vale        string   `json:"vale_version"`
	Version     string   `json:"version"`

	// Releases lists the package's releases and the Vale each needs, so
	// sync can pick one without walking a host's feed. See pkgver.go.
	Releases []Release `json:"releases"`
}

// A Release is one entry of a package's release manifest.
type Release struct {
	Version string `json:"version"`
	Vale    string `json:"vale_version"`
	URL     string `json:"url"`
}

// FetchJSON returns the body of a GET request to url.
func FetchJSON(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Fetch downloads the zip archive at src and unpacks it into dst.
func Fetch(src, dst string) error {
	// Fetch the resource from the web:
	resp, err := http.Get(src) //nolint:gosec,noctx

	if err != nil {
		return err
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not fetch '%s' (status code '%d')", src, resp.StatusCode)
	}

	// Create a temp file to represent the archive locally:
	tmpfile, err := os.CreateTemp("", "temp.*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name()) // clean up

	// Write to the  local archive:
	_, err = io.Copy(tmpfile, resp.Body)
	if err != nil {
		return err
	} else if err = tmpfile.Close(); err != nil {
		return err
	}

	resp.Body.Close()
	return system.Unarchive(tmpfile.Name(), dst)
}

// Version is the running Vale's version. The `vale` binary sets it at start
// from its release build; "master" means a development build, which accepts
// any package. Library users may set it to the Vale version they target.
var Version = "master"
