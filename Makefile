GO ?= go

.PHONY: all build test vet fmt check netns clean

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

clean:
	rm -f pktc
	$(GO) clean ./...

demo:
	@for f in testdata/*.lisp; do \
	  printf '%-24s ' "$$(basename $$f)"; \
	  $(GO) run ./cmd/pktc lower -quiet $$f | head -1; \
	done