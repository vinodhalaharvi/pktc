package command

import "testing"

func TestQuoting(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"ip", "ip"},
		{"10.0.0.1/24", "10.0.0.1/24"},
		{"gre-1_2", "gre-1_2"},
		{"2001:db8::1", "2001:db8::1"},
		{"a b", "'a b'"},
		{"", "''"},
		{"it's", `'it'\''s'`},
	}
	for _, tt := range tests {
		if got := quote(tt.in); got != tt.want {
			t.Errorf("quote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestPlainAndShell(t *testing.T) {
	s := Script{
		New("make it", "ip", "link", "add", "gre1"),
		New("", "ip", "link", "set", "gre1", "up"),
	}
	if got, want := s.Plain(), "ip link add gre1\nip link set gre1 up"; got != want {
		t.Errorf("Plain() = %q, want %q", got, want)
	}
	if got, want := s.Shell(), "# make it\nip link add gre1\n\nip link set gre1 up\n"; got != want {
		t.Errorf("Shell() = %q, want %q", got, want)
	}
}
