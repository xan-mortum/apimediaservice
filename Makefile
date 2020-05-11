# Default binary name
BINARY ?= apimediaservice

GO ?= go
BIN ?= bin
BIN_DIR ?= ./$(BIN)

check-bin:
	@mkdir -p $(BIN_DIR)
.PHONY: check-bin

generate:
	mkdir -p gen
	swagger generate server -t gen -f ./swagger.yml --exclude-main
.PHONY: generate

start:
	@$(BIN_DIR)/$(BINARY)
.PHONY: start

build: check-bin
	@echo "==> building $(BINARY) binary"
	@$(GO) build -o $(BIN_DIR)/$(BINARY)
.PHONY: build

check-swagger:
	which swagger || (GO111MODULE=off go get -u github.com/go-swagger/go-swagger/cmd/swagger)

swagger: check-swagger
	GO111MODULE=on go mod vendor  && GO111MODULE=off swagger generate spec -o ./swagger.yml --scan-models

serve-swagger: check-swagger
	swagger serve -F=swagger swagger.yml