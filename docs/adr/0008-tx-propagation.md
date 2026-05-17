# ADR 0008: tx 伝搬 (context.Context + UnaryTxInterceptor)

- Status: Accepted (2026-05-17)
- 関連 ADR: 0003 (DI 配線、 C08 で予定) / 0006 (DB schema 分離、 同 DB なので tx 共有可能) / 0007 (sqlc、 生成コードへの tx 受け渡し方)
- 関連 (TS 版): [0010 AsyncLocalStorage で tx を ambient に持つ](../../../ec-modular-monolith-prototype-ts/docs/adr/0010-als-tx-context.md)

## Context

複数 BC を 1 つの DB tx に束ねる需要が Step 5 (Orders) で必ず発生する (`placeOrder` が members / products / inventory / payments を直列で叩く)。 各 repository / usecase の引数に `tx` を引き回す案は単純だが:

- handler / orchestration で `(tx, input)` のボイラーが増殖する
- BC 内 usecase はほぼ全関数が `tx` を第一引数に取ることになり、 unit test の見通しが下がる
- 「テストで tx 渡さず呼ぶ」 「本番で tx 経由で呼ぶ」 の切替が型に出ない (`tx` のモック必須)

TS 版は同じ問題を `AsyncLocalStorage` (ALS) で解決した (ADR 0010 TS)。 Go では言語標準で同等のことを `context.Context` で行える。 gRPC は **すべての RPC で ctx を第一引数に運ぶ規約** が確立しており、 interceptor で ctx を加工できる。

## Decision drivers

- 「BC 横断 tx」 が必須 (Step 5)
- handler / usecase 配線のボイラーを最小化したい
- usecase / repository は tx 引数なしで書け、 unit test で tx を意識しないで済むようにしたい
- gRPC の interceptor pattern と相性が良い
- TS 版 (ALS) と意味論的に対応させて 「ambient な tx context」 の比較ができる

## Decision

### tx を `context.Context` に載せる

- `internal/shared/tx/context.go` に `WithTx(ctx, pgx.Tx) ctx` / `GetTx(ctx) pgx.Tx` を置く
- `WithTx` は **interceptor 専用**。 application code から直接呼ばない
- `GetTx(ctx)` は **repository 実装からのみ** 呼ぶ (usecase / domain は触らない)
- ctx に tx が入っていなければ `nil`。 fail-fast にしない理由は後述の Trade-offs

### gRPC interceptor で全 unary RPC を tx スコープに包む

`internal/shared/tx/middleware.go` の `UnaryTxInterceptor(pool *pgxpool.Pool)` が:

1. RPC の冒頭で `pool.BeginTx(ctx, pgx.TxOptions{})` を実行
2. `WithTx(ctx, tx)` で派生 ctx を作って handler に渡す
3. handler 成功時に `tx.Commit(ctx)`
4. handler エラー / panic / commit 失敗時に `tx.Rollback(context.Background())` (cancelled ctx でも rollback できるよう Background)

```go
func UnaryTxInterceptor(pool *pgxpool.Pool) grpc.UnaryServerInterceptor {
    return func(ctx, req, info, h) (resp any, err error) {
        tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
        if err != nil { return nil, err }
        committed := false
        defer func() { if !committed { _ = tx.Rollback(context.Background()) } }()
        resp, err = h(WithTx(ctx, tx), req)
        if err != nil { return nil, err }
        if err = tx.Commit(ctx); err != nil { return nil, err }
        committed = true
        return resp, nil
    }
}
```

### 各層の責務

- **interceptor**: tx 開始 / Commit / Rollback、 ctx への load
- **repository** (infra): `GetTx(ctx)` で tx を取り出し `db.New(tx)` で sqlc Queries を構築。 ここだけが tx を意識
- **usecase**: tx を意識しない。 ctx だけ受け取って repository に渡す
- **domain**: tx も ctx も知らない (純ドメイン)
- **handler**: tx を意識しない。 ctx だけ usecase に渡す

### BC 横断 tx (Step 5 の placeOrder)

1 RPC = 1 tx という interceptor の規約上、 placeOrder という 1 つの RPC の中で `members.FindById(ctx, id)` `inventory.Reserve(ctx, ...)` を順に呼べば、 ctx 経由で同じ tx に束ねられる。 BC 横断であっても 「同じ DB」 「同じ tx」 を共有できるのが schema 分離 (B2、 ADR 0006) の前提と整合する。

### Read-only RPC でも tx を張る

`health.Ping` のような DB アクセス無しの RPC でも interceptor が tx を作る。 これは無駄ではあるが:

- 「特殊な RPC を例外扱い」 で interceptor の挙動が分岐するより、 シンプルに 1 RPC = 1 tx を貫く方が運用しやすい
- 全 RPC で同じ tx 規約になり、 repository が ctx に tx が無いケースを考慮しなくて済む
- 読み取り専用 tx (`AccessMode: ReadOnly`) は将来オプションとして検討余地あり (今は全て read/write)

### streaming RPC は Step 0+1 では対象外

streaming RPC は long-lived 接続で 「1 RPC = 1 tx」 が破綻する。 Step 0+1 では unary 一本で進め、 必要になった時点で別 interceptor (`StreamTxInterceptor`) を別 ADR で設計。

## Alternatives considered

### 各関数の引数で `tx` を explicit に引き回す
- 棄却理由:
  - usecase / repository の全関数が `tx` を持ち、 placeOrder で各 BC 呼び出しに毎回 `tx, ...` を書く必要
  - test での mock 渡しが手間 (現状の構成なら fakeTx を一切渡さず unit test できる)
  - TS 版で同じ案を採って ALS に変えた経緯 (TS ADR 0010) を踏襲

### `database/sql.Tx` を ctx ではなく goroutine local に持つ
- 棄却理由:
  - Go には goroutine local 変数の仕組みが意図的に無い (公式の方針)
  - 自前実装すると goroutine ID 取得など黒魔術になる
  - ctx で十分

### handler が `withTx(func(tx) {...})` を明示で呼ぶ
- 棄却理由:
  - handler ごとに同じボイラーを書く
  - 「tx 忘れ」 が静的に検出できない (interceptor で全包の方が抜け漏れ無し)

### `pgxpool` ではなく `database/sql`
- 棄却理由:
  - pgx 直結の方が型の細やかさ・パフォーマンスとも上、 sqlc も pgx/v5 を推奨

## Trade-offs

良い面:
- usecase / domain が tx を一切知らない (純粋に保てる)
- 1 RPC = 1 tx の暗黙ルールで BC 横断 tx が "自然に" 動く
- placeOrder で `tx` のボイラーが消える
- interceptor で集約しているため、 tx の開始 / 終了 / rollback の挙動を 1 箇所で変更可能

悪い面 / コスト:
- repository が `GetTx(ctx)` でランタイムに tx を取り出す形になり、 「tx が無い ctx で呼ばれる」 ケースが型で防げない (実行時に nil 返却)
- ctx に何が積まれているかが grep しないと見えない (副作用 invisibility)
- Read-only RPC でも tx を開始するため、 健康診断のような軽量 RPC でも DB 往復 1 回ぶんのオーバーヘッド
- 単体テスト で repository を直接呼ぶ場合は `WithTx(ctx, fakeTx)` を手で組む必要がある (interceptor を回さないので)

## When to revisit

- streaming RPC を実装する時: 別 interceptor (`StreamTxInterceptor`) 設計、 1 stream 内の tx 寿命は要再検討
- Read-only RPC のオーバーヘッドが運用上のコストになった時: `pgx.TxOptions{AccessMode: pgx.ReadOnly}` への切替 / interceptor の opt-out 機構
- BC 横断オーケストレーションを REST gateway 経由で実装する必要が出た時 (Step N): REST → gRPC client → 内部 server の経路で tx 境界をどこに置くか別 ADR
- 並列 fan-out (1 RPC 内で複数 goroutine が同 tx を共有) が必要になった時: pgx.Tx は単一 goroutine 前提なので別 ADR

## References

- pgx pgxpool: <https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool>
- grpc interceptors: <https://pkg.go.dev/google.golang.org/grpc#UnaryServerInterceptor>
- TS 版 ADR 0010 (ALS で tx): `../../../ec-modular-monolith-prototype-ts/docs/adr/0010-als-tx-context.md`
