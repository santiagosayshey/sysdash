.PHONY: dev dev-frontend dev-backend build clean

# Development
dev:
	@cd backend && go mod tidy
	@trap 'kill 0' EXIT; \
	cd backend && go run . & \
	cd frontend && npm run dev

dev-frontend:
	cd frontend && npm run dev

dev-backend:
	cd backend && go run .

# Build frontend and copy to backend/static
build-frontend:
	cd frontend && npm run build
	rm -rf backend/static
	cp -r frontend/dist backend/static

# Build backend binary (includes embedded frontend)
build: build-frontend
	cd backend && go build -o ../sysdash .

# Build for multiple platforms
build-linux-amd64: build-frontend
	cd backend && GOOS=linux GOARCH=amd64 go build -o ../sysdash-linux-amd64 .

build-linux-arm64: build-frontend
	cd backend && GOOS=linux GOARCH=arm64 go build -o ../sysdash-linux-arm64 .

build-all: build-frontend
	cd backend && GOOS=linux GOARCH=amd64 go build -o ../sysdash-linux-amd64 .
	cd backend && GOOS=linux GOARCH=arm64 go build -o ../sysdash-linux-arm64 .
	cd backend && GOOS=darwin GOARCH=amd64 go build -o ../sysdash-darwin-amd64 .
	cd backend && GOOS=darwin GOARCH=arm64 go build -o ../sysdash-darwin-arm64 .

# Clean build artifacts
clean:
	rm -rf frontend/dist backend/static sysdash sysdash-*

# Install dependencies
deps:
	cd frontend && npm install
	cd backend && go mod tidy
