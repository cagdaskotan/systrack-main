# SysTrack Makefile

.PHONY: build run seed migrate-up migrate-down clean test dev

# Build the application
build:
	go build -o systrack ./cmd/systrack

# Run the application
run:
	go run ./cmd/systrack

# Seed admin user
seed:
	go run ./scripts/seed_admin.go

# Run database migrations up
migrate-up:
	migrate -path internal/db/migrations -database "mysql://root:CHANGE_ME_ROOT_PASS@tcp(localhost:3306)/systrack" up

# Run database migrations down
migrate-down:
	migrate -path internal/db/migrations -database "mysql://root:CHANGE_ME_ROOT_PASS@tcp(localhost:3306)/systrack" down

# Clean build artifacts
clean:
	rm -f systrack

# Run tests
test:
	go test ./...

# Development mode with hot reload
dev:
	air

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Install development tools
install-tools:
	go install github.com/cosmtrek/air@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Setup development environment
setup: install-tools deps
	@echo "Development environment setup complete!"
	@echo "Run 'make migrate-up' to setup database"
	@echo "Run 'make seed' to create admin user"
	@echo "Run 'make run' to start the server"
