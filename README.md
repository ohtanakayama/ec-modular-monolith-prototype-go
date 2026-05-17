# ec-modular-monolith-prototype-go

EC 共通バックエンド基盤の感触を確かめるためのプロトタイプ。Go / gRPC / Postgres でモジュラーモノリスを組む。

姉妹リポジトリ: [ec-modular-monolith-prototype-ts](https://github.com/ohtanakayama/ec-modular-monolith-prototype-ts) (TypeScript / Hono / Prisma / MySQL / REST)。同じ題材 (5 BC: 会員・商品・在庫・決済・注文) を別技術で組み直し、設計判断の比較を成立させる。

## 目的

このプロトタイプで答えを出したい問い:

1. **gRPC と REST 混在のルーティング・バリデーション内部設計**
   公開 API (REST) と内部サービス間通信 (gRPC) が並走する時、翻訳層 (handler ↔ usecase の mapping) をどう設計するか。プロキシ分離 / プロセス分離 / ポート分離の妥当性を検証する。
2. **モジュール単位の DB 分離レベル**
   Postgres の schema 分離 (BC ごとに schema) を採用し、TS 版の単一スキーマ運用と比較。schema 跨ぎクエリ (Composite Read Model) の見た目と運用負荷を測る。
3. **DDD がこの粒度で適しているか**
   TS と等条件の薄め DDD を Go で書き、Go の package 可視性で Aggregate 境界をコンパイラレベルで強制できるかを試す。
4. **副次: Go / TS / PHP のプロセスモデル差の体感**
   PHP (リクエスト寿命・shared-nothing) と Go (長寿命プロセス・goroutine + context) の違いを、tx 伝搬・DI 配線・BC 境界の側面から実感する。

## 技術スタック

| | |
|---|---|
| Language | Go 1.22+ |
| API (内部) | gRPC (`google.golang.org/grpc`) |
| API (公開) | REST / OpenAPI (Step N で導入予定) |
| proto / codegen | `buf` |
| DB | Postgres 16 (Docker Compose) |
| DB アクセス | `sqlc` (SQL-first) |
| Migration | `golang-migrate` |
| Tx 伝搬 | `context.Context` + gRPC `UnaryTxInterceptor` |
| 設定 | `envconfig` |
| ログ | `log/slog` (構造化、標準ライブラリ) |
| テスト | `go test` + `testify` |
| 境界強制 | Go package 可視性 (+ Step 2 で `depguard` 投入予定) |
| アーキテクチャ | モジュラーモノリス + レイヤード + 一部 CQRS (注文 BC、Step 6 以降) |

## アーキテクチャ概要

```
[ブラウザ] [外部 webhook] [SaaS 利用者]
     │            │            │
     └────────────┼────────────┘
                  ▼
        ┌──────────────────────────┐
        │  REST 公開ゲートウェイ    │   ← Step N で生やす
        │  (OpenAPI-first)         │
        └──────────────────────────┘
                  │ gRPC
                  ▼
        ┌──────────────────────────┐
        │  内部 gRPC モノリス       │   ← Step 0+1 で構築
        │  Members / Products /     │
        │  Inventory / Payments /   │
        │  Orders + Orchestration   │
        └──────────────────────────┘
                  │
              Postgres (schema 分離)
```

- 全 BC は内部 gRPC でのみ公開、BC ごとの公開/非公開分けはしない
- 公開 REST は用途別 (ブラウザ / webhook / SaaS) に Step N で追加
- Step 0+1 は **1 プロセス・1 ポート (gRPC のみ)**
- DB は **BC ごとに schema 分離** (`members.member`, `products.product` のように schema-qualified)

### Bounded Context (DDD) でモジュール化

```
internal/modules/
  members/      会員 BC      ← Step 1 で実装
  products/    商品 BC      ← Step 2 で実装予定
  inventory/   在庫 BC      ← Step 3
  payments/    決済 BC      ← Step 4
  orders/      注文 BC      ← Step 5+
```

各 BC は内部で 4 層に分割:

```
gRPC handler → usecase → domain → infra (sqlc)
```

### tx 伝搬: context.Context + UnaryTxInterceptor

TS 版の `AsyncLocalStorage` (ADR 0010) に相当する役割を、Go では `context.Context` で実現する。gRPC interceptor で全 unary call を tx スコープに包む:

```go
// internal/shared/tx/middleware.go
func UnaryTxInterceptor(pool *pgxpool.Pool) grpc.UnaryServerInterceptor {
  return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
    tx, _ := pool.Begin(ctx)
    ctx = withTx(ctx, tx)
    res, err := h(ctx, req)
    if err != nil { tx.Rollback(ctx); return nil, err }
    return res, tx.Commit(ctx)
  }
}
```

repository は `tx.GetTx(ctx)` で interceptor が用意した tx を取り出す。usecase / handler は tx を意識しない。

### 公開境界 = `internal/modules/<bc>/module.go`

BC 外から触って良いのは `module.go` で公開される DI モジュールのみ。他 BC の `usecase` / `domain` / `infra` への直接参照は Step 2 で `depguard` により CI でブロックされる予定。現状 (Step 0+1) は Go の package 可視性 (大文字 / 小文字) で表現。

### Aggregate 境界の機械的強制

```go
// internal/modules/orders/domain/order/order.go (package: order)
type Order struct {
  id     string
  items  []*orderItem   // 小文字 = package 外から不可視
  status Status
}

type orderItem struct { ... }   // package 外から見えない
```

Aggregate root (`Order`) 経由でしか子要素 (`orderItem`) に触れない、を Go の package 可視性で **コンパイラレベル** で強制。TS 版で ESLint flat config で機械化していた境界が、Go では言語ネイティブで成立する。

## ディレクトリ構造

```
ec-modular-monolith-prototype-go/
├── cmd/
│   ├── internal-server/        # gRPC モノリス (Step 0)
│   └── rest-gateway/           # 公開 REST ゲートウェイ (Step N)
├── proto/                      # buf 管理
│   ├── buf.yaml
│   ├── buf.gen.yaml
│   ├── health/v1/
│   └── members/v1/             # Step 1
├── openapi/                    # 公開 OpenAPI 契約 (Step N)
├── migrations/                 # golang-migrate (BC ごとの schema/table 作成)
├── internal/
│   ├── modules/<bc>/
│   │   ├── domain/<aggregate>/
│   │   ├── usecase/
│   │   ├── infra/
│   │   │   ├── queries/        # sqlc 入力 SQL
│   │   │   └── db/             # sqlc 生成コード
│   │   ├── grpc/
│   │   └── module.go
│   └── shared/
│       ├── tx/                 # tx middleware + context
│       └── errors/             # typed error 4 種
├── tests/
│   └── integration/            # 実 DB での e2e
├── docs/adr/                   # アーキテクチャ判断記録
├── sqlc.yaml
├── buf.work.yaml               # (proto/ 直下に buf.yaml 配置)
├── docker-compose.yml
├── Makefile
└── go.mod
```

## ローカルセットアップ

事前準備 (一度だけ):

```bash
# Go ツールチェーン (Step 0 で必要)
go install github.com/bufbuild/buf/cmd/buf@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 動作確認用
brew install grpcurl   # macOS
```

起動:

```bash
cp .env.example .env
docker compose up -d
make migrate-up
make buf-gen
make sqlc-gen
make build
./bin/internal-server
```

別ターミナルで:

```bash
grpcurl -plaintext localhost:9090 list
# → grpc.health.v1.Health, grpc.reflection.v1.ServerReflection が見えれば Step 0 成功
```

## 実装ステップ (ロードマップ)

| Step | 内容 | 状態 |
|---|---|---|
| 0 | 土台構築 (proto / sqlc / migration / tx / gRPC server) | 進行中 |
| 1 | Members BC を gRPC で一周 (Register / GetById) | 未着手 |
| 2 | Products BC + 境界 linter (`depguard`) 投入 | 未着手 |
| 3 | Inventory BC (reserve / commit / release の状態機械) | 未着手 |
| 4 | Payments BC (Gateway Port + mock) | 未着手 |
| 5 | Orders BC (write) + 全 BC 横断 tx | 未着手 |
| 6 | Orders BC (read) + Composite Read Model (schema 跨ぎ JOIN) | 未着手 |
| 7 | Orders キャンセル補償 | 未着手 |
| N | 公開 REST gateway (OpenAPI + `oapi-codegen` + RFC 7807) | 未着手 |

各 Step 完了時に `docs/adr/` に判断記録を追加し、`git tag step-X-complete` で区切る。

## TS 版との設計差分

| 観点 | TS 版 | Go 版 |
|---|---|---|
| 言語 / FW | TypeScript / Hono | Go / grpc-go (+ Echo は Step N) |
| DB | MySQL 8 / 単一スキーマ | **Postgres 16 / schema 分離** |
| DB アクセス | Prisma (ORM) | **sqlc (SQL-first)** |
| API 形式 | REST のみ | **gRPC のみ** (REST は Step N) |
| API 契約 | OpenAPI-first | **proto-first** (buf) |
| tx 伝搬 | `AsyncLocalStorage` | **`context.Context` + interceptor** |
| BC 公開境界 | `public/index.ts` + ESLint 強制 | `module.go` + Go package 可視性 |
| Aggregate 境界 | 曖昧 (実行時のみ) | **package 可視性でコンパイラ強制** |
| エラー | class-based 例外 4 種 | **typed error 4 種** |
| DI | 関数カリー化 + manual wire | 同じ |
| テスト | vitest unit + integration | go test + testify |

## 設計判断記録 (ADR)

`docs/adr/` 配下に置く。Step 0 で 10 件、以降は新規判断が出るたびに追加。

## 既知の限界 (現段階)

- まだ Step 0 のため、業務ロジックは実装されていない
- 公開 REST は未実装 (Step N で導入)
- 境界 linter は未投入 (Step 2 で `depguard`)
- 観測性 (OpenTelemetry / Prometheus) は未導入
- CI (GitHub Actions) は未設定
