SHELL := powershell.exe

.PHONY: docker-up docker-down docker-rebuild docker-test lint

lint:
	golangci-lint run

test:
	go test ./...

docker-up:
	docker-compose up -d --build

docker-down:
	docker-compose down -v

docker-rebuild:
	docker-compose down -v
	docker-compose up -d --build