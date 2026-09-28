.PHONY: boundary-check check fmt race test vet

check: boundary-check vet race

boundary-check:
	@if go list -deps ./... | rg -q '^github.com/mytecor/r1s(/|$$)'; then \
		echo "meshbus must not depend on r1s packages"; \
		exit 1; \
	fi
	@if go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' . ./realm \
		| rg -v '^$$|^github.com/mytecor/meshbus(/realm)?$$' | rg -q .; then \
		echo "meshbus core and realm must depend only on the Go standard library"; \
		exit 1; \
	fi

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './**/testdata/*')

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...
