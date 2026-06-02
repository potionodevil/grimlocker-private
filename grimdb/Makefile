.PHONY: test build lint security integration

build:
	cd sdk/rust && cargo build
	cd sdk/csharp && dotnet build
	cd sdk/typescript && npm run build

test:
	go test ./... -v -race
	cd sdk/rust && cargo test
	cd sdk/csharp && dotnet test
	cd sdk/typescript && npm test
	cd sdk/python && pytest
	cd sdk/java && mvn test
	cd sdk/php && vendor/bin/phpunit
	cd sdk/ruby && rake test
	cd sdk/swift && swift test
	cd sdk/kotlin && gradle test
	cd sdk/dart && dart test

lint:
	golangci-lint run ./...
	cd sdk/rust && cargo clippy -- -D warnings
	cd sdk/typescript && npx tsc --noEmit
	cd sdk/python && ruff check .
	cd sdk/php && vendor/bin/phpstan analyse src

security:
	go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...
	gosec ./...

integration:
	go test ./tests/... -v -tags=integration
