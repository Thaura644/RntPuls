.PHONY: build test test-coverage lint vet run clean docker-up docker-down

build:
	CGO_ENABLED=0 go build -o bin/rentpulse ./cmd/api

test:
	go test -race -count=1 ./...

test-coverage:
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	go vet ./...

vet:
	go vet ./...

run:
	go run ./cmd/api

clean:
	rm -rf bin/ coverage.out frontend/dist/ frontend/node_modules/

docker-up:
	docker compose up --build

docker-down:
	docker compose down
