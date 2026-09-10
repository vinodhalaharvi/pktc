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
	"github.com/vinodhalaharvi/pktc/mirror"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
	"github.com/vinodhalaharvi/pktc/tiles/linux"
	"github.com/vinodhalaharvi/pktc/wire"
)

const usage = `pktc — describe the packet, derive the configuration

usage:
  pktc parse <file>              print the encapsulation spine
  pktc lower [flags] <file>      print the commands that produce it
  pktc mirror [flags] <file>     print both ends of the tunnel
  pktc expect <file>             print what the underlay should carry

lower flags:
  -underlay <dev>   physical device the outermost tunnel attaches to (default eth0)
  -quiet            omit explanatory comments
  -key-dir <dir>    where a script looks for key material (default /etc/wireguard)

mirror flags:
  -underlay <dev>       underlay on the near end (default eth0)
  -peer-underlay <dev>  underlay on the far end (defaults to -underlay)
  -peer-inner <cidr>    the far end's inner address; without it the peer
                        script omits the address and says so

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
	case "mirror":
		err = cmdMirror(os.Args[2:])
	case "expect":
		err = cmdExpect(os.Args[2:])
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

// readTrees is the shared front end: read, build, validate. A file may
// hold several rules, because a machine rarely has exactly one tunnel.
func readTrees(path string) ([]*ast.Node, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	forms, err := sexp.Read(path, string(src))
	if err != nil {
		return nil, err
	}
	if len(forms) == 0 {
		return nil, fmt.Errorf("%s: no forms found", path)
	}
	out := make([]*ast.Node, 0, len(forms))
	for _, f := range forms {
		n, err := ast.Build(f)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// readTree is for commands that operate on a single rule.
func readTree(path string) (*ast.Node, error) {
	trees, err := readTrees(path)
	if err != nil {
		return nil, err
	}
	if len(trees) != 1 {
		return nil, sexp.Errorf(trees[1].Pos,
			"this command takes one (configure ...) form; %s holds %d. Use 'lower' for a whole file",
			path, len(trees))
	}
	return trees[0], nil
}

func readSpine(path string) (spine.Spine, error) {
	root, err := readTree(path)
	if err != nil {
		return spine.Spine{}, err
	}
	return spine.FromForm(root)
}

// render lowers one tree against a shared Env and prints it.
func render(heading string, root *ast.Node, e *env.Env, quiet bool) error {
	e.Reset()
	sp, err := spine.FromForm(root)
	if err != nil {
		return err
	}
	reg, err := linux.Registry()
	if err != nil {
		return err
	}
	res, err := reg.Cover(sp, e)
	if err != nil {
		return err
	}
	if quiet {
		fmt.Println(res.Script.Plain())
		return nil
	}
	fmt.Printf("#!/bin/sh\n")
	if heading != "" {
		fmt.Printf("# %s\n", heading)
	}
	fmt.Printf("# %s\n# tiles: %s\n\n", sp, strings.Join(res.Applied, " \u2192 "))
	fmt.Print(res.Script.Shell())
	if res.Payload.Len() > 0 {
		fmt.Printf("\n# %s below the tunnel is traffic, not configuration\n", res.Payload)
	}
	return nil
}

func cmdMirror(args []string) error {
	fs := flag.NewFlagSet("mirror", flag.ExitOnError)
	underlay := fs.String("underlay", "eth0", "underlay on the near end")
	peerUnderlay := fs.String("peer-underlay", "", "underlay on the far end")
	peerInner := fs.String("peer-inner", "", "the far end's inner address")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	if *peerUnderlay == "" {
		*peerUnderlay = *underlay
	}

	root, err := readTree(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := render("near end", root, env.New(*underlay), false); err != nil {
		return err
	}

	res, err := mirror.Peer(root, *peerInner)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("# " + strings.Repeat("-", 68))
	for _, s := range res.Supplied {
		fmt.Println("# derived: " + s)
	}
	for _, m := range res.Missing {
		fmt.Println("# NOT DERIVED: " + m)
	}
	fmt.Println("# " + strings.Repeat("-", 68))
	fmt.Println()

	if err := render("far end", res.Tree, env.New(*peerUnderlay), false); err != nil {
		return err
	}
	if len(res.Missing) > 0 {
		fmt.Println("\n# the far end is incomplete; supply -peer-inner to finish it")
	}
	return nil
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
	keyDir := fs.String("key-dir", env.DefaultKeyDir, "where a script looks for key material")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	trees, err := readTrees(fs.Arg(0))
	if err != nil {
		return err
	}
	// One Env for the whole file: the rules are configured on one
	// machine, so a device name claimed by one is unavailable to the next.
	e := env.New(*underlay)
	e.KeyDir = *keyDir
	for i, root := range trees {
		if i > 0 && !*quiet {
			fmt.Println()
		}
		heading := ""
		if len(trees) > 1 {
			heading = fmt.Sprintf("rule %d of %d", i+1, len(trees))
		}
		if err := render(heading, root, e, *quiet); err != nil {
			return err
		}
	}
	return nil
}

// cmdExpect reads the same tree a third way: not as commands, and not
// turned around, but as a prediction of the wire. Capture the underlay
// once the configuration is running and the two should agree.
func cmdExpect(args []string) error {
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	sp, err := readSpine(args[0])
	if err != nil {
		return err
	}
	exp, err := wire.Expect(sp)
	if err != nil {
		return err
	}
	fmt.Print(exp)
	return nil
}

func describe(n *ast.Node) string {
	parts := make([]string, 0, len(n.Props))
	for _, p := range n.Props {
		parts = append(parts, fmt.Sprintf(":%s %s", p.Key, p.Val.Text))
	}
	return strings.Join(parts, "  ")
}
