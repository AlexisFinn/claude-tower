package titles

import "testing"

func TestIsDefaultName(t *testing.T) {
	tests := []struct {
		name, cwd string
		want      bool
	}{
		{"zest-f6", "/home/u/zest", true},
		{"zest-5c", "/home/u/zest/", true},
		{"zest-0123abcd", "/home/u/zest", true},
		{"zest-f", "/home/u/zest", false},
		{"zest-0123abcde", "/home/u/zest", false},
		{"zest-F6", "/home/u/zest", false},
		{"zest-g6", "/home/u/zest", false},
		{"zest-", "/home/u/zest", false},
		{"zest", "/home/u/zest", false},
		{"zest-f6-suite", "/home/u/zest", false},
		{"other-f6", "/home/u/zest", false},
		{"refonte export", "/home/u/zest", false},
		{"my.proj [x]-ab", "/home/u/my.proj [x]", true},
		{"a+b(c)-ab", "/home/u/a+b(c)", true},
		{"a.b-ab", "/home/u/axb", false},
		{"-ab", "/", false},
		{"", "/home/u/zest", true},
	}
	for _, tt := range tests {
		if got := IsDefaultName(tt.name, tt.cwd); got != tt.want {
			t.Errorf("IsDefaultName(%q, %q) = %v, want %v", tt.name, tt.cwd, got, tt.want)
		}
	}
}
