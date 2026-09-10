GO ?= go

.PHONY: all build test vet fmt check netns demo mirror-demo nested-demo expect-demo explain-demo clean

all: check

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

fmt:
	gofmt -w .

check: vet test
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "not gofmt'd:"; echo "$$unformatted"; exit 1; fi

# Run the generated configuration against a real kernel. Linux only,
# needs root, and skips whatever the kernel cannot provide.
netns:
	sudo -E $$(which $(GO)) test -tags netns -v ./...

# Both ends of a tunnel, derived from one description.
mirror-demo:
	@$(GO) run ./cmd/pktc mirror -peer-inner 10.100.0.2/24 -peer-underlay wan testdata/vxlan.lisp

# A tunnel inside a tunnel: two tiles, one spine, no combined tile.
nested-demo:
	@$(GO) run ./cmd/pktc lower testdata/vxlan-nested.lisp

# The third reading of one tree: what the underlay should carry.
expect-demo:
	@$(GO) run ./cmd/pktc expect testdata/vxlan.lisp

# Why that tile, and what stopped the others.
explain-demo:
	@$(GO) run ./cmd/pktc explain testdata/vlan-vxlan.lisp

clean:
	rm -f pktc
	$(GO) clean ./...

demo:
	@for f in testdata/*.lisp; do \
	  printf '%-24s ' "$$(basename $$f)"; \
	  $(GO) run ./cmd/pktc lower -quiet $$f | head -1; \
	done