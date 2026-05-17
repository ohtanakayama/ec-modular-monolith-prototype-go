.PHONY: help build run test test-unit test-integration tidy fmt lint \
        migrate-up migrate-down migrate-status migrate-force \
        sqlc-gen buf-gen buf-lint buf-breaking \
        up down logs psql clean

# ----- defaults ---------------------------------------------------------------

GO            ?= go
APP_NAME      ?= internal-server
BIN_DIR       ?= bin
DATABASE_URL  ?= postgres://ec_user:ec_pass@localhost:5432/ec_modular?sslmode=disable
MIGRATE_DIR   ?= migrations

# ----- meta -------------------------------------------------------------------

help: ## このヘルプ
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ----- build / run ------------------------------------------------------------

build: ## internal-server をビルド
	$(GO) build -o $(BIN_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)

run: build ## internal-server を起動
	./$(BIN_DIR)/$(APP_NAME)

tidy: ## go.mod / go.sum を整理
	$(GO) mod tidy

fmt: ## gofmt
	$(GO) fmt ./...

# ----- test -------------------------------------------------------------------

test: ## 全テスト (unit + integration)
	$(GO) test ./...

test-unit: ## unit テスト (tests/integration を除外)
	$(GO) test $$($(GO) list ./... | grep -v '/tests/integration')

test-integration: ## integration テスト (要 Postgres 起動)
	$(GO) test ./tests/integration/...

# ----- proto / sqlc -----------------------------------------------------------

buf-gen: ## proto → Go コード生成
	cd proto && buf generate

buf-lint: ## proto の lint
	cd proto && buf lint

buf-breaking: ## proto の breaking change チェック (main 基準)
	cd proto && buf breaking --against '../.git#branch=main,subdir=proto'

sqlc-gen: ## SQL → Go コード生成
	sqlc generate

# ----- docker compose ---------------------------------------------------------

up: ## Postgres コンテナ起動
	docker compose up -d

down: ## Postgres コンテナ停止
	docker compose down

logs: ## Postgres のログ
	docker compose logs -f postgres

psql: ## ローカル Postgres に psql で接続
	docker compose exec postgres psql -U ec_user -d ec_modular

# ----- migrate ----------------------------------------------------------------

migrate-up: ## migration を全て適用
	migrate -path $(MIGRATE_DIR) -database "$(DATABASE_URL)" up

migrate-down: ## migration を 1 つ巻き戻し
	migrate -path $(MIGRATE_DIR) -database "$(DATABASE_URL)" down 1

migrate-status: ## migration 適用状況
	migrate -path $(MIGRATE_DIR) -database "$(DATABASE_URL)" version

migrate-force: ## migration を強制バージョン指定 (引数 VERSION=N)
	migrate -path $(MIGRATE_DIR) -database "$(DATABASE_URL)" force $(VERSION)

# ----- cleanup ----------------------------------------------------------------

clean: ## ビルド成果物を削除
	rm -rf $(BIN_DIR)
