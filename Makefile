.PHONY: all build run test coverage lint clean docker-build docker-up docker-down docker-logs

BINARY_NAME=bin/api

all: build

build:
	@mkdir -p bin
	go build -o $(BINARY_NAME) ./cmd/api

run: build
	./$(BINARY_NAME)

test:
	go test -v -race ./...

coverage:
	go test -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	go vet ./...

clean:
	rm -rf bin/ coverage.out

docker-build:
	docker compose build

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f
