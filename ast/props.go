package ast

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/vinodhalaharvi/pktc/sexp"
)

// The property vocabulary. Two definitions may share a key with
// different types: :src is a bare address on an outer layer and a
// prefix on an inner one, and each reader asks for what it needs.
var (
	Src       = PropDef[netip.Addr]{"src", ParseAddr}
	Dst       = PropDef[netip.Addr]{"dst", ParseAddr}
	SrcPrefix = PropDef[netip.Prefix]{"src", ParsePrefix}
	DstPrefix = PropDef[netip.Prefix]{"dst", ParsePrefix}

	SrcPort = PropDef[uint16]{"src-port", parseU16}
	DstPort = PropDef[uint16]{"dst-port", parseU16}

	VNI      = PropDef[uint32]{"vni", parseVNI}
	Key      = PropDef[uint32]{"key", parseU32}
	Sequence = PropDef[uint32]{"sequence", parseU32}
	SPI      = PropDef[uint32]{"spi", parseU32}

	Protocol = PropDef[string]{"protocol", ParseText}
	Mode     = PropDef[string]{"mode", ParseText}
	Name     = PropDef[string]{"name", ParseText}
)

// ParseAddr accepts a bare IP address, quoted or not.
func ParseAddr(v sexp.Value) (netip.Addr, error) {
	if v.Kind != sexp.KString && v.Kind != sexp.KSymbol {
		return netip.Addr{}, fmt.Errorf("expected an IP address, got %s", v.Kind)
	}
	a, err := netip.ParseAddr(v.Text)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%q is not an IP address", v.Text)
	}
	return a, nil
}

// ParsePrefix accepts address/length. A bare address is rejected: the
// prefix length is information the packet cannot carry, so it has to be
// written down rather than guessed.
func ParsePrefix(v sexp.Value) (netip.Prefix, error) {
	if v.Kind != sexp.KString && v.Kind != sexp.KSymbol {
		return netip.Prefix{}, fmt.Errorf("expected address/length, got %s", v.Kind)
	}
	if !strings.Contains(v.Text, "/") {
		return netip.Prefix{}, fmt.Errorf(
			"%q needs a prefix length (for example %s/24); a packet does not carry one", v.Text, v.Text)
	}
	p, err := netip.ParsePrefix(v.Text)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q is not a valid address/length", v.Text)
	}
	return p, nil
}

// ParseText accepts a string or a bare symbol.
func ParseText(v sexp.Value) (string, error) {
	if v.Kind != sexp.KString && v.Kind != sexp.KSymbol {
		return "", fmt.Errorf("expected a name, got %s", v.Kind)
	}
	return v.Text, nil
}

type unsigned interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}

func parseUnsigned[T unsigned](v sexp.Value, bits int) (T, error) {
	var zero T
	if v.Kind != sexp.KNumber {
		return zero, fmt.Errorf("expected a number, got %s", v.Kind)
	}
	text, base := v.Text, 10
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		text, base = text[2:], 16
	}
	u, err := strconv.ParseUint(text, base, bits)
	if err != nil {
		return zero, fmt.Errorf("%s does not fit in %d bits", v.Text, bits)
	}
	return T(u), nil
}

func parseU16(v sexp.Value) (uint16, error) { return parseUnsigned[uint16](v, 16) }
func parseU32(v sexp.Value) (uint32, error) { return parseUnsigned[uint32](v, 32) }

func parseVNI(v sexp.Value) (uint32, error) {
	n, err := parseUnsigned[uint32](v, 32)
	if err != nil {
		return 0, err
	}
	if n > 0xFFFFFF {
		return 0, fmt.Errorf("%d exceeds the 24-bit VNI range (0..16777215)", n)
	}
	return n, nil
}
