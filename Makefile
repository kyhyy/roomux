BINARY := roomux
BINDIR := bin

.PHONY: build run clean vet

build:
	go build -o $(BINDIR)/$(BINARY) ./cmd/roomux

run:
	go run ./cmd/roomux

vet:
	go vet ./...

clean:
	rm -rf $(BINDIR) dist
