.PHONY: all server agent agents web clean dev-certs run tidy fmt vet

GO       ?= go
BIN      := bin
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

all: server agent web

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

server:
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN)/service-edge ./cmd/server

agent:
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN)/agent ./cmd/agent

# Cross-compile agent binaries for the platforms agents run on.
agents:
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags '$(LDFLAGS)' -o $(BIN)/agent_linux_amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags '$(LDFLAGS)' -o $(BIN)/agent_linux_arm64 ./cmd/agent

web:
	cd web && npm install && npm run build

# Generate a self-signed dev CA for local testing.
dev-certs:
	$(GO) run ./cmd/server gen-ca --out dev

run: server
	$(BIN)/service-edge --config config.yaml

clean:
	rm -rf $(BIN) web/dist
