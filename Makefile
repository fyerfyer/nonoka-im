GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always)

ifeq ($(GOHOSTOS), windows)
	#the `find.exe` is different from `find` in bash/shell.
	#to see https://docs.microsoft.com/en-us/windows-server/administration/windows-commands/find.
	#changed to use git-bash.exe to run find cli or other cli friendly, caused of every developer has a Git.
	#Git_Bash= $(subst cmd\,bin\bash.exe,$(dir $(shell where git)))
	Git_Bash=$(subst \,/,$(subst cmd\,bin\bash.exe,$(dir $(shell where git))))
	INTERNAL_PROTO_FILES=$(shell $(Git_Bash) -c "find internal -name *.proto")
	API_PROTO_FILES=$(shell $(Git_Bash) -c "find api -name *.proto")
else
	INTERNAL_PROTO_FILES=$(shell find internal -name *.proto)
	API_PROTO_FILES=$(shell find api -name *.proto)
endif

.PHONY: init
# init env
init:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
	go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest
	go install github.com/google/wire/cmd/wire@latest

.PHONY: config
# generate internal proto
config:
	protoc --proto_path=./internal \
	       --proto_path=./third_party \
 	       --go_out=paths=source_relative:./internal \
	       $(INTERNAL_PROTO_FILES)

.PHONY: api
# generate api proto
api:
	protoc --proto_path=./api \
	       --proto_path=./third_party \
 	       --go_out=paths=source_relative:./api \
 	       --go-http_out=paths=source_relative:./api \
 	       --go-grpc_out=paths=source_relative:./api \
	       --openapi_out=fq_schema_naming=true,default_response=false:. \
	       $(API_PROTO_FILES)

.PHONY: test-deps-up test-deps-down
# start test dependencies (postgres, redis, kafka)
test-deps-up:
	@docker compose -f docker-compose.test.yml up -d
	@echo "Waiting for services to be ready..."
	@sleep 5

# stop test dependencies
test-deps-down:
	@docker compose -f docker-compose.test.yml down -v

.PHONY: test-auth
# run auth integration tests
test-auth:
	@go test -v ./test/integration/... -run 'TestAuth' -count=1 -timeout 30s

.PHONY: test-gateway
# run gateway basic integration tests (no concurrent, no kafka)
test-gateway:
	@go test -v ./test/integration/... -run 'TestGateway_' -skip 'TestGateway_(Concurrent|Kafka|MixedTraffic|Broadcast_Concurrent|Connection_Reliability|RedisSession)' -count=1 -timeout 60s

.PHONY: test-gateway-concurrent
# run gateway concurrent / stress / reliability tests
test-gateway-concurrent:
	@go test -v ./test/integration/... -run 'TestGateway_(Concurrent|MixedTraffic|Broadcast_Concurrent|Connection_Reliability|RedisSession)' -count=1 -timeout 60s

.PHONY: test-kafka
# run kafka integration tests (requires test-deps-up)
test-kafka:
	@go test -v ./test/integration/... -run 'TestGateway_Kafka' -count=1 -timeout 180s

.PHONY: test
# run all integration tests (start deps, run tests by group, cleanup)
test: test-deps-up
	@echo "\n========================================"
	@echo "=== Running Auth Tests               ==="
	@echo "========================================"
	@go test -v ./test/integration/... -run 'TestAuth' -count=1 -timeout 30s || { $(MAKE) test-deps-down; exit 1; }
	@echo "\n========================================"
	@echo "=== Running Gateway Tests            ==="
	@echo "========================================"
	@go test -v ./test/integration/... -run 'TestGateway_' -skip 'TestGateway_(Concurrent|Kafka|MixedTraffic|Broadcast_Concurrent|Connection_Reliability|RedisSession)' -count=1 -timeout 60s || { $(MAKE) test-deps-down; exit 1; }
	@echo "\n========================================"
	@echo "=== Running Gateway Concurrent Tests ==="
	@echo "========================================"
	@go test -v ./test/integration/... -run 'TestGateway_(Concurrent|MixedTraffic|Broadcast_Concurrent|Connection_Reliability|RedisSession)' -count=1 -timeout 60s || { $(MAKE) test-deps-down; exit 1; }
	@echo "\n========================================"
	@echo "=== Running Kafka Tests              ==="
	@echo "========================================"
	@go test -v ./test/integration/... -run 'TestGateway_Kafka' -count=1 -timeout 180s || { $(MAKE) test-deps-down; exit 1; }
	@echo "\n========================================"
	@echo "=== All tests passed!                ==="
	@echo "========================================"
	@$(MAKE) test-deps-down

.PHONY: build
# build
build:
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/ ./...

.PHONY: demo-scale
# start the scale demo with three independently scalable msgworkers
demo-scale:
	docker compose -f docker-compose.yml -f docker-compose.scale.yml up -d --build --scale msgworker=3

.PHONY: generate
# generate
generate:
	go generate ./...
	go mod tidy

.PHONY: all
# generate all
all:
	make api
	make config
	make generate

# show help
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
