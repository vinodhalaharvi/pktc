package sexp

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		text string
		want Kind
	}{
		{"ipv4", KSymbol},
		{":vni", KKeyword},
		{"*", KStar},
		{"100", KNumber},
		{"0x1001", KNumber},
		{"-1", KNumber},
		{"10.0.0.1", KSymbol}, // dotted forms are not numbers
		{"0x", KSymbol},
		{"true", KSymbol},
	}
	for _, tt := range tests {
		if got := classify(tt.text); got != tt.want {
			t.Errorf("classify(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestReadOK(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		forms int
	}{
		{"atom form", `(ipv4)`, 1},
		{"props", `(udp :dst-port 4789)`, 1},
		{"nested", `(ipv4 (udp (vxlan)))`, 1},
		{"comment", "; note\n(ipv4)", 1},
		{"trailing comment", "(ipv4) ; note", 1},
		{"two forms", `(a) (b)`, 2},
		{"star", `(vxlan :vni *)`, 1},
		{"string escape", `(x :name "a\"b")`, 1},
		{"empty", ``, 0},
		{"only comment", `; nothing`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Read("t.lisp", tt.src)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if len(got) != tt.forms {
				t.Fatalf("got %d forms, want %d", len(got), tt.forms)
			}
		})
	}
}

func TestReadErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"unclosed", `(ipv4`},
		{"unexpected close", `)`},
		{"extra close", `(ipv4))`},
		{"unterminated string", `(x :n "abc`},
		{"bad escape", `(x :n "a\qb")`},
		{"bare colon", `(x : 1)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Read("t.lisp", tt.src); err == nil {
				t.Fatalf("expected an error for %q", tt.src)
			}
		})
	}
}

func TestPositions(t *testing.T) {
	src := "(ipv4\n  (udp))"
	forms, err := Read("t.lisp", src)
	if err != nil {
		t.Fatal(err)
	}
	outer := forms[0]
	if outer.Pos.Line != 1 || outer.Pos.Col != 1 {
		t.Errorf("outer at %s, want t.lisp:1:1", outer.Pos)
	}
	inner := outer.Items[1]
	if inner.Pos.Line != 2 || inner.Pos.Col != 3 {
		t.Errorf("inner at %s, want t.lisp:2:3", inner.Pos)
	}
}

func TestStringEscapes(t *testing.T) {
	forms, err := Read("t.lisp", `(x :n "a\"b\\c\nd")`)
	if err != nil {
		t.Fatal(err)
	}
	got := forms[0].Items[2].Text
	want := "a\"b\\c\nd"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
