package scip

import "testing"

func TestProjectRootToLocalPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Regression for https://github.com/scip-code/scip/issues/282 —
		// Windows ProjectRoot of the form "file://X:\..." used to fail
		// url.Parse with "invalid port" because "X:" looked like host:port.
		{
			name: "windows file uri with drive letter and backslashes",
			in:   `file://D:\dev\project`,
			want: `D:\dev\project`,
		},
		// Unix/macOS forms must keep their existing behavior unchanged.
		{
			name: "unix file uri",
			in:   "file:///test/project",
			want: "/test/project",
		},
		{
			name: "unix file uri single slash",
			in:   "file:/root",
			want: "/root",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ProjectRootToLocalPath(tc.in)
			if err != nil {
				t.Fatalf("ProjectRootToLocalPath(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ProjectRootToLocalPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
