TEST_DB ?= postgres://postgres@127.0.0.1:5432/bhagirath_test?sslmode=disable

.PHONY: build test lint run up
build:
	go build -o bin/bhagirath ./cmd/bhagirath
test:
	BHAGIRATH_TEST_DATABASE_URL="$(TEST_DB)" go test -race -count=1 ./...
lint:
	test -z "$$(gofmt -l .)" && go vet ./...
run: build
	./bin/bhagirath serve
up:
	docker compose up --build
