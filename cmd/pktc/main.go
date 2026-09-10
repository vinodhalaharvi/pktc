// Command pktc compiles packet-shaped S-expressions.
//
// Milestone 0 ships the front end only: read, validate, and report the
// encapsulation spine. Lowering to link configuration comes next.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
)

const usage = `pktc — describe the packet, derive the configuration

usage:
  pktc parse <file>    read a .lisp file and print its encapsulation spine

pktc emits configuration; it never applies it.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "parse":
		if len(os.Args) != 3 {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		if err := parse(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "pktc: "+err.Error())
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "pktc: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func parse(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	forms, err := sexp.Read(path, string(src))
	if err != nil {
		return err
	}
	switch len(forms) {
	case 1:
	case 0:
		return fmt.Errorf("%s: no forms found", path)
	default:
		return sexp.Errorf(forms[1].Pos,
			"this version compiles one (configure ...) form per file; found %d", len(forms))
	}

	root, err := ast.Build(forms[0])
	if err != nil {
		return err
	}
	sp, err := spine.FromForm(root)
	if err != nil {
		return err
	}

	fmt.Printf("spine: %s\n\n", sp)
	width := 0
	for _, n := range sp.Layers {
		if len(n.Name) > width {
			width = len(n.Name)
		}
	}
	for _, n := range sp.Layers {
		fmt.Printf("%s\n", strings.TrimRight(fmt.Sprintf("  %-*s  %s", width, n.Name, describe(n)), " "))
	}
	return nil
}

func describe(n *ast.Node) string {
	if len(n.Props) == 0 {
		return ""
	}
	parts := make([]string, 0, len(n.Props))
	for _, p := range n.Props {
		parts = append(parts, fmt.Sprintf(":%s %s", p.Key, p.Val.Text))
	}
	return strings.Join(parts, "  ")
}
