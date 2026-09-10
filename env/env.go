// Package env holds the deployment facts a packet tree cannot carry.
//
// Prefix lengths, device names, MTU, and the underlay interface are all
// required by the target and absent from the packet. This is the same
// role a calling convention plays in a compiler back end.
package env

import (
	"fmt"
	"strings"
)

// MaxIfName is the Linux interface name limit, including the
// terminating NUL: names must be 15 characters or fewer.
const MaxIfName = 15

// Env carries deployment facts and allocates interface names.
type Env struct {
	// Root is the physical device the outermost tunnel attaches to.
	Root string

	// Dev is the device the next tile should attach to. It starts as
	// Root and advances as each tile creates a device, which is how a
	// tunnel inside a tunnel finds its underlay.
	Dev string

	// Created lists every device made so far, outermost first. A
	// chained stack has more than one, and all of them need bringing
	// up, not just the innermost.
	Created []string

	used  map[string]bool
	count map[string]int
}

// New builds an Env rooted at a physical device.
func New(root string) *Env {
	return &Env{
		Root:  root,
		Dev:   root,
		used:  map[string]bool{root: true},
		count: map[string]int{},
	}
}

// Claim reserves an exact interface name.
func (e *Env) Claim(name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	if e.used[name] {
		return "", fmt.Errorf("interface name %q is already used by another rule", name)
	}
	e.used[name] = true
	return name, nil
}

// Alloc reserves the next free name with the given prefix: gre1, gre2.
func (e *Env) Alloc(prefix string) (string, error) {
	for {
		e.count[prefix]++
		name := fmt.Sprintf("%s%d", prefix, e.count[prefix])
		if err := validName(name); err != nil {
			return "", err
		}
		if !e.used[name] {
			e.used[name] = true
			return name, nil
		}
	}
}

// Reset returns to the physical device, keeping every name already
// taken. Rules in one file are configured on one machine, so they share
// a namespace of interface names but each starts from the underlay.
func (e *Env) Reset() { e.Dev = e.Root }

// Enter records that a tile created dev, so the next tile attaches to
// it. This is the whole of the chaining mechanism: an outer tile's
// device becomes an inner tile's underlay, and nothing else has to know
// that a stack was nested.
func (e *Env) Enter(dev string) {
	e.Created = append(e.Created, dev)
	e.Dev = dev
}

func validName(name string) error {
	if name == "" {
		return fmt.Errorf("interface name is empty")
	}
	if len(name) > MaxIfName {
		return fmt.Errorf("interface name %q is %d characters; Linux allows %d",
			name, len(name), MaxIfName)
	}
	if strings.ContainsAny(name, " /\t\n") {
		return fmt.Errorf("interface name %q contains an illegal character", name)
	}
	return nil
}
