.PHONY: build run test clean docker-build docker-run fmt

# Build the binary
build:
	CGO_ENABLED=1 go build -o bin/scryarr ./cmd/worker

# Run locally with example config
run: build
	./bin/scryarr --config ./config/app.yml --categories ./config/categories.yml

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f data/scryarr.sqlite
	rm -f data/recommendations/*.json
	rm -f output/*.yml

# Format code
fmt:
	go fmt ./...
	go mod tidy

# Build Docker image
docker-build:
	docker build -t scryarr:latest .

# Run Docker container locally
docker-run:
	docker run --rm \
		-v $(PWD)/config:/config \
		-v $(PWD)/data:/data \
		-v $(PWD)/output:/output \
		-e TAUTULLI_API_KEY=${TAUTULLI_API_KEY} \
		-e PLEX_TOKEN=${PLEX_TOKEN} \
		-e TMDB_API_KEY=${TMDB_API_KEY} \
		-e LLM_API_BASE=${LLM_API_BASE} \
		-e LLM_API_KEY=${LLM_API_KEY} \
		-p 8080:8080 \
		scryarr:latest

# Install dependencies
deps:
	go mod download
	go mod tidy

# Lint code
lint:
	go vet ./...
