all: test

test:
	go test -buildmode=default -race -cover -covermode=atomic ./...
