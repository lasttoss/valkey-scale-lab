.PHONY: test mutate up down measure write chart diagram image

test: ## every gate that does not need a server
	@go vet ./...
	@go test ./... -count=1

# A gate nobody has seen fail is a gate nobody can trust. Each mutation is a plausible mistake and
# each one has to make the suite red; the file is restored from a copy and the copy is compared
# afterwards, because "the tests pass again" is not the same as "the file is back".
mutate: ## break the fix on purpose; every mutation must be caught
	@python3 scripts/mutate.py

up: ## start the server the lab talks to (host port 6479)
	@docker compose up -d --wait

down: ## stop it
	@docker compose down -v

measure: up ## reproduce the incident and write docs/measurements.md
	@go run ./cmd/lab -mode leak  -matches 300 -out /tmp/lab-leak300.md
	@go run ./cmd/lab -mode leak  -matches 600 -out /tmp/lab-leak600.md || true
	@go run ./cmd/lab -mode fixed -matches 300 -pool 16 -out /tmp/lab-fixed300.md
	@go run ./cmd/lab -mode fixed -matches 600 -pool 16 -out /tmp/lab-fixed600.md
	@go run ./cmd/lab -mode fixed -matches 50 -keys 200000 -sweeps 20 -walk keys -out /tmp/lab-keys.md
	@go run ./cmd/lab -mode fixed -matches 50 -keys 200000 -sweeps 20 -walk scan -out /tmp/lab-scan.md
	@python3 scripts/write-measurements.py \
	  /tmp/lab-leak300.md /tmp/lab-leak600.md /tmp/lab-fixed300.md /tmp/lab-fixed600.md /tmp/lab-keys.md /tmp/lab-scan.md \
	  > docs/measurements.md
	@echo "docs/measurements.md regenerated"

image: ## build the lab image
	@docker build -q -t valkey-scale-lab:dev .

chart: ## lint, render both modes, and validate
	@./scripts/chart-check.sh

diagram: ## regenerate the figure and check it against its sources
	@python3 docs/figures/valkey-scale-lab-flow.generator.py
	@bash ~/.pi/agent/scripts/diagram-freshness.sh 2>/dev/null | grep -E "FRESH|STALE|TOUCHED|NO-META|UNCOMMITTED" | sed 's/^/  /'
