APP_PORT ?= 8493

.PHONY: run test build docker-build docker-run clean

run:
	go run ./cmd/localcreds
	ENGLANDSOFTWARE_PORT=$(APP_PORT) go run .

test:
	go test ./...

build:
	go build -o englandsoftware .

docker-build:
	docker build -t englandsoftware:local .

docker-run: docker-build
	docker run --rm -p $(APP_PORT):$(APP_PORT) -e ENGLANDSOFTWARE_PORT=$(APP_PORT) -v $(CURDIR)/config:/app/config -v $(CURDIR)/data:/app/data englandsoftware:local

clean:
	rm -f englandsoftware
