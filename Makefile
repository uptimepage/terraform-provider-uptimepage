BINARY := terraform-provider-uptimepage

.PHONY: build test fmt vet check tidy

build:
	go build ./...

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

# Pre-commit gate: format check + vet + build + unit tests.
check:
	test -z "$$(gofmt -l .)" || (echo "gofmt needed:" && gofmt -l . && exit 1)
	go vet ./...
	go build ./...
	go test ./...

tidy:
	go mod tidy
