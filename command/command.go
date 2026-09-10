// Package command holds emitted configuration as structured values.
//
// Commands are argv slices rather than strings so that tests compare
// arguments instead of whitespace, and so a second backend (netlink,
// UCI, gNMI) is a new renderer rather than a rewrite.
package command

import (
	"strings"
)

// Command is one unit of configuration, with the reason it exists.
type Command struct {
	Argv []string
	Why  string
}

// New builds a command with an explanation.
func New(why string, argv ...string) Command {
	return Command{Argv: argv, Why: why}
}

// String renders the command as a shell line.
func (c Command) String() string {
	parts := make([]string, len(c.Argv))
	for i, a := range c.Argv {
		parts[i] = quote(a)
	}
	return strings.Join(parts, " ")
}

// Script is an ordered set of commands. Order matters: these are not
// commutative, and a tunnel cannot be created before its underlay.
type Script []Command

// Shell renders the script, one command per line, each preceded by the
// reason it exists.
func (s Script) Shell() string {
	var b strings.Builder
	for i, c := range s {
		if i > 0 {
			b.WriteByte('\n')
		}
		if c.Why != "" {
			b.WriteString("# " + c.Why + "\n")
		}
		b.WriteString(c.String() + "\n")
	}
	return b.String()
}

// Plain renders the commands with no comments.
func (s Script) Plain() string {
	lines := make([]string, len(s))
	for i, c := range s {
		lines[i] = c.String()
	}
	return strings.Join(lines, "\n")
}

// safe reports whether a token needs no quoting.
func safe(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			c >= '0' && c <= '9' ||
			c == '.' || c == '/' || c == ':' || c == '-' || c == '_'
		if !ok {
			return false
		}
	}
	return true
}

func quote(s string) string {
	if safe(s) {
		return s
	}
	// A command substitution has to reach the shell intact. Double
	// quotes keep it a single word without disabling it; single quotes
	// would turn a key lookup into a literal string.
	if strings.Contains(s, "$(") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
