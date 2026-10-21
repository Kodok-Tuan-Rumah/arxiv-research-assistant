.PHONY: run run-ml test lint tidy up down ps setup migrate ingest ablation health build

# ── Dev ────────────────────────────────────────────────────────────────────────
run:
	cd go-backend && go run cmd/server/main.go

run-ml:
	cd python-ml && source venv/bin/activate && uvicorn main:app --host 0.0.0.0 --port 8001 --reload

# ── Infrastructure ─────────────────────────────────────────────────────────────
up:
	docker compose -f infra/docker-compose.yml up -d

down:
	docker compose -f infra/docker-compose.yml down

ps:
	docker compose -f infra/docker-compose.yml ps

# ── Setup (one-command from fresh clone) ──────────────────────────────────────
setup:
	@echo "🐳 Starting Docker services..."
	docker compose -f infra/docker-compose.yml up -d
	@echo "⏳ Waiting for PostgreSQL..."
	@sleep 8
	@echo "🗄️  Running migrations..."
	docker exec -i $$(docker ps --filter "name=postgres" --format "{{.Names}}") \
		psql -U arxiv -d arxiv_db < go-backend/migrations/001_base.up.sql
	docker exec -i $$(docker ps --filter "name=postgres" --format "{{.Names}}") \
		psql -U arxiv -d arxiv_db < go-backend/migrations/002_query_logs.up.sql
	docker exec -i $$(docker ps --filter "name=postgres" --format "{{.Names}}") \
		psql -U arxiv -d arxiv_db < go-backend/migrations/003_versioning.up.sql
	@echo "🐍 Setting up Python venv..."
	cd python-ml && python3 -m venv venv && venv/bin/pip install -r requirements.txt -q
	@echo "🔨 Building Go backend..."
	cd go-backend && go mod tidy && go build ./...
	@echo ""
	@echo "✅ Setup complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  Terminal 1: make run-ml"
	@echo "  Terminal 2: make run"
	@echo "  Terminal 3: make ingest"

migrate:
	docker exec -i $$(docker ps --filter "name=postgres" --format "{{.Names}}") \
		psql -U arxiv -d arxiv_db < go-backend/migrations/001_base.up.sql
	docker exec -i $$(docker ps --filter "name=postgres" --format "{{.Names}}") \
		psql -U arxiv -d arxiv_db < go-backend/migrations/002_query_logs.up.sql
	docker exec -i $$(docker ps --filter "name=postgres" --format "{{.Names}}") \
		psql -U arxiv -d arxiv_db < go-backend/migrations/003_versioning.up.sql

# ── Tools ──────────────────────────────────────────────────────────────────────
ingest:
	curl -s -X POST http://localhost:8001/ingest \
		-H "Content-Type: application/json" \
		-d '{"arxiv_ids": ["2312.10997"]}' | python3 -m json.tool

health:
	curl -s http://localhost:8080/health | python3 -m json.tool

ablation:
	bash scripts/ablation_study.sh

# ── Go ─────────────────────────────────────────────────────────────────────────
test:
	cd go-backend && go test ./... -v

test-ml:
	cd python-ml && PYTHONPATH=. pytest tests/ -v

lint:
	cd go-backend && go vet ./...

tidy:
	cd go-backend && go mod tidy

build:
	cd go-backend && go build ./...
