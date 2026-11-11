// Package fsutil holds the path-confinement helpers shared by the scanner and
// the file-serving API. Library paths come from the database, but they are
// still treated as untrusted: nothing may escape the library root.
package fsutil

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrOutside is returned when a path would leave its library root.
var ErrOutside = errors.New("path escapes the library root")

// Within reports whether p is root itself or lies inside it. Both paths must
// already be absolute and cleaned.
func Within(root, p string) bool {
	root, p = filepath.Clean(root), filepath.Clean(p)
	if p == root {
		return true
	}
	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}
	return strings.HasPrefix(p, root)
}

// CleanRel validates a slash-separated relative path as stored in the
// database and returns it cleaned. "" and "." both mean "the root itself".
func CleanRel(rel string) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", ErrOutside
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	if strings.HasPrefix(rel, "/") {
		return "", ErrOutside
	}
	c := path.Clean(rel)
	if c == "." {
		return "", nil
	}
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", ErrOutside
	}
	return c, nil
}

// ResolveUnder joins root and the relative path rel and returns the real file
// path. Unless follow is set, symbolic links are resolved and the result must
// still be inside the (resolved) root; with follow set, links may point
// anywhere but rel itself still cannot use ".." to leave root.
func ResolveUnder(root, rel string, follow bool) (string, error) {
	c, err := CleanRel(rel)
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(c))
	if follow {
		return full, nil
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", err
	}
	if !Within(rootReal, real) {
		return "", ErrOutside
	}
	return real, nil
}

// IsRegular stats the file and reports whether it is a regular file.
func IsRegular(p string) (os.FileInfo, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	return fi, nil
}
