package env

import "testing"

func TestAllocSequential(t *testing.T) {
	e := New("eth0")
	for _, want := range []string{"gre1", "gre2", "gre3"} {
		got, err := e.Alloc("gre")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestAllocSkipsClaimed(t *testing.T) {
	e := New("eth0")
	if _, err := e.Claim("gre1"); err != nil {
		t.Fatal(err)
	}
	got, err := e.Alloc("gre")
	if err != nil {
		t.Fatal(err)
	}
	if got != "gre2" {
		t.Errorf("got %q, want gre2", got)
	}
}

func TestClaimRejectsDuplicate(t *testing.T) {
	e := New("eth0")
	if _, err := e.Claim("vxlan100"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Claim("vxlan100"); err == nil {
		t.Error("a second claim of the same name must fail")
	}
	if _, err := e.Claim("eth0"); err == nil {
		t.Error("the underlay name must not be reusable")
	}
}

func TestNameLengthLimit(t *testing.T) {
	e := New("eth0")
	if _, err := e.Claim("this-name-is-far-too-long"); err == nil {
		t.Error("names longer than 15 characters must be rejected")
	}
}

func TestEnterAdvancesDev(t *testing.T) {
	e := New("eth0")
	if e.Dev != "eth0" {
		t.Fatalf("Dev starts at %q, want eth0", e.Dev)
	}
	e.Enter("xfrm0")
	if e.Dev != "xfrm0" {
		t.Errorf("Dev = %q after Enter, want xfrm0", e.Dev)
	}
	if e.Root != "eth0" {
		t.Errorf("Root changed to %q; it must stay the physical device", e.Root)
	}
}
