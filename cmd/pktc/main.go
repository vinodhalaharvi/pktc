// Command pktc compiles packet-shaped S-expressions into link
// configuration.
//
// You describe the packet you expect on the wire; pktc works out which
// kernel primitive produces it, by tiling the encapsulation spine the
// way a compiler back end selects instructions.
//
// pktc prints configuration. It never applies it.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
	"github.com/vinodhalaharvi/pktc/tiles/linux"
)

const usage = `pktc — describe the packet, derive the configuration

usage:
  pktc parse <file>              print the encapsulation spine
  pktc lower [flags] <file>      print the commands that produce it

lower flags:
  -underlay <dev>   physical device the outermost tunnel attaches to (default eth0)
  -quiet            omit explanatory comments

pktc prints configuration; it never applies it.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "parse":
		err = cmdParse(os.Args[2:])
	case "lower":
		err = cmdLower(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "pktc: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pktc: "+err.Error())
		os.Exit(1)
	}
}

// readSpine is the shared front end: read, build, validate, flatten.
func readSpine(path string) (spine.Spine, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return spine.Spine{}, err
	}
	forms, err := sexp.Read(path, string(src))
	if err != nil {
		return spine.Spine{}, err
	}
	switch len(forms) {
	case 1:
	case 0:
		return spine.Spine{}, fmt.Errorf("%s: no forms found", path)
	default:
		return spine.Spine{}, sexp.Errorf(forms[1].Pos,
			"this version compiles one (configure ...) form per file; found %d", len(forms))
	}
	root, err := ast.Build(forms[0])
	if err != nil {
		return spine.Spine{}, err
	}
	return spine.FromForm(root)
}

func cmdParse(args []string) error {
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	sp, err := readSpine(args[0])
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
		line := fmt.Sprintf("  %-*s  %s", width, n.Name, describe(n))
		fmt.Println(strings.TrimRight(line, " "))
	}
	return nil
}

func cmdLower(args []string) error {
	fs := flag.NewFlagSet("lower", flag.ExitOnError)
	underlay := fs.String("underlay", "eth0", "physical device the outermost tunnel attaches to")
	quiet := fs.Bool("quiet", false, "omit explanatory comments")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	sp, err := readSpine(fs.Arg(0))
	if err != nil {
		return err
	}
	reg, err := linux.Registry()
	if err != nil {
		return err
	}
	res, err := reg.Cover(sp, env.New(*underlay))
	if err != nil {
		return err
	}

	if *quiet {
		fmt.Println(res.Script.Plain())
	} else {
		fmt.Printf("#!/bin/sh\n# %s\n# tiles: %s\n\n",
			sp, strings.Join(res.Applied, " → "))
		fmt.Print(res.Script.Shell())
		if res.Payload.Len() > 0 {
			fmt.Printf("\n# %s below the tunnel is traffic, not configuration\n", res.Payload)
		}
	}
	return nil
}

func describe(n *ast.Node) string {
	parts := make([]string, 0, len(n.Props))
	for _, p := range n.Props {
		parts = append(parts, fmt.Sprintf(":%s %s", p.Key, p.Val.Text))
	}
	return strings.Join(parts, "  ")
}
