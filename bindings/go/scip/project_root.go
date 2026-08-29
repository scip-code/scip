package scip

import "net/url"

// ProjectRootToLocalPath converts an Index.Metadata.ProjectRoot value into a
// local filesystem path.
//
// It handles the Windows form "file://X:\..." (where X is a drive letter)
// which net/url.Parse rejects with `invalid port ":\..." after host`, because
// it parses "X:" as a host:port pattern. See issue #282.
//
// For all other inputs it preserves the previous behavior of using
// url.Parse(rawProjectRoot).Path.
func ProjectRootToLocalPath(rawProjectRoot string) (string, error) {
	if isWindowsFileURIWithDriveLetter(rawProjectRoot) {
		// Strip the "file://" prefix; the remainder is the Windows path.
		// ponytail: only the "file://X:..." form is handled here; the
		// canonical "file:///X:/..." form parses fine via url.Parse and is
		// out of scope for this fix.
		return rawProjectRoot[len("file://"):], nil
	}
	u, err := url.Parse(rawProjectRoot)
	if err != nil {
		return "", err
	}
	return u.Path, nil
}

// isWindowsFileURIWithDriveLetter reports whether s matches "file://X:..."
// where X is an ASCII letter and the position after the scheme is NOT a third
// slash (which would indicate the canonical "file:///..." form).
func isWindowsFileURIWithDriveLetter(s string) bool {
	const prefix = "file://"
	if len(s) < len(prefix)+2 {
		return false
	}
	if s[:len(prefix)] != prefix {
		return false
	}
	c := s[len(prefix)]
	if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') {
		return false
	}
	return s[len(prefix)+1] == ':'
}
