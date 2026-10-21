ES_URL ?= http://localhost:9200

.PHONY: up down reset logs ps health run tidy

up:
	docker compose up -d

down:
	docker compose down

reset:
	docker compose down -v

logs:
	docker compose logs -f elasticsearch

ps:
	docker compose ps

health:
	curl -fsS "$(ES_URL)/_cluster/health?pretty"

run:
	go run ./cmd/api

tidy:
	go mod tidy
