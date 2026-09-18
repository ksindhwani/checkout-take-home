.PHONY: build test test-integration run docker-up docker-down swagger-gen

build:
	go build ./...

test:
	go test ./... -race

test-integration:
	go test -tags=integration ./internal/bank/...

run:
	go run main.go

docker-up:
	docker-compose up --build

docker-down:
	docker-compose down

swagger-gen:
	swag init -g main.go --output docs
