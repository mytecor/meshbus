.PHONY: boundary-check check cover fmt fuzz-smoke race test vet

check: boundary-check vet race

boundary-check:
	@if go list -deps ./... | grep -Eq '^github.com/mytecor/r1s(/|$$)'; then \
		echo "meshbus must not depend on r1s packages"; \
		exit 1; \
	fi
	@if go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' . ./realm \
		| grep -Ev '^$$|^github.com/mytecor/meshbus(/realm)?$$' | grep -q .; then \
		echo "meshbus core and realm must depend only on the Go standard library"; \
		exit 1; \
	fi

cover:
	go test -coverprofile=coverage.out ./...

fuzz-smoke:
	go test -run='^$$' -fuzz=FuzzDecodeEvent -fuzztime=5s .
	go test -run='^$$' -fuzz=FuzzAuthMessageUnpack -fuzztime=5s ./rns
	go test -run='^$$' -fuzz=FuzzParsePresence -fuzztime=5s ./rns

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './**/testdata/*')

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...
