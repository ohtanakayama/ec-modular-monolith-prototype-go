# ADR 0007: DB アクセス層に sqlc を採用 (SQL-first)

- Status: Accepted (2026-05-17)
- 関連 ADR: 0006 (DB schema 分離、 後続予定) / 0008 (tx 伝搬、 C07 で予定)
- 関連 (TS 版): TS では Prisma (ORM) を採用。 本プロトの 「TS vs Go」 比較軸の一つ

## Context

Go で Postgres にアクセスする層の選択肢は大別して 3 系統:

1. **生 `database/sql` / `pgx` 直書き**
2. **ORM**: GORM / ent / Bun
3. **SQL-first コード生成**: sqlc

本プロトタイプは TS 版で Prisma (ORM) を採っており、 Go 版では別アプローチを選んで設計差分を比較できることが価値になる。 加えて schema 分離 (PG schema を BC ごと、 ADR 0006) を採るため、 schema-qualified なクエリ (`SELECT * FROM members.member`) を素直に書けることが要件。

## Decision drivers

- TS 版 (Prisma / ORM 寄り) と意図的に対比できる
- schema 分離との相性 (schema-qualified なクエリを生で書ける)
- domain 層から SQL 詳細を遮蔽する仕組み (repository port パターン) を阻害しない
- 型安全性 (実行時エラーではなくコンパイル時)
- DDL は migration で別管理、 sqlc が DDL を所有しない

## Decision

### sqlc を採用

- 各 BC は `internal/modules/<bc>/infra/queries/*.sql` に query を書く
- sqlc は `internal/modules/<bc>/infra/db/` に Go コードを生成 (`Queries` 型と関数群)
- 生成された型は **infra 層に閉じる**。 domain には漏らさず、 repository 実装で domain entity に変換
- ドライバは `pgx/v5` (`sql_package: "pgx/v5"`)

### sqlc.yaml の構造

- `version: "2"`
- BC ごとに 1 つの `sql` エントリ
- `schema:` は `migrations/` をまとめて指定 (migration が全 BC の schema を兼ねる)
- `queries:` は BC 配下のディレクトリのみ
- `gen.go.out:` は BC 配下 (`internal/modules/<bc>/infra/db`)

```yaml
# Step 1 で members を追加したときの形
sql:
  - engine: "postgresql"
    queries: "internal/modules/members/infra/queries"
    schema:  "migrations"
    gen:
      go:
        package: "db"
        out:     "internal/modules/members/infra/db"
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
```

### Step 0 では空 (`sql: []`)

`health` サービスは DB アクセスを持たないため、 C05 段階では `sql.yaml` を空配列にして雛形コメントだけ置く。 Step 1 (C13) で members 用の 1 エントリを追加して `sqlc generate` を回す。

### 生成物の扱い

- `internal/modules/<bc>/infra/db/` 配下の生成 Go ファイルは **git に commit**
- 理由は ADR 0004 (proto の生成物) と同じ: `go build` が外部ツール無しで通る、 PR diff で SQL 変更の影響が見える
- CI で `make sqlc-gen` + `git diff --exit-code` を Step 1 以降に追加し、 生成漏れを検出

### tx 伝搬との接続

sqlc の生成コードは `*sql.DB` / `*pgx.Conn` / `pgx.Tx` のいずれも受け取れる。 本プロトでは repository 実装が `tx.GetTx(ctx)` で interceptor が ctx に積んだ `pgx.Tx` を取り出し、 `db.New(tx)` で `*Queries` を構築する形に統一する (詳細は ADR 0008)。

## Alternatives considered

### `database/sql` / `pgx` 直書き
- 棄却理由:
  - 引数 / 結果型を手書きしてズレるリスクを毎回背負う
  - SQL とコードの間で型整合性が無く、 リファクタ時に壊れやすい
  - 「型安全性」 という Go の強みを生かしきれない

### GORM
- 棄却理由:
  - ORM 経由で書くと生 SQL が読みにくくなる。 schema 分離との相性も悪い (table のメタデータ管理が複雑)
  - クエリ最適化が見えづらく、 N+1 などが発生してから気づく
  - TS の Prisma と性質が近く、 「比較軸」 として面白くない

### ent
- 棄却理由:
  - スキーマを Go コードで定義する独自の DSL。 学習コストが高め
  - migration を ent が所有する形になりやすく、 sqlc + golang-migrate のように「migration は別ツール、 DB アクセスは別ツール」 の分業ができない
  - schema 分離との相性は要検証 (採用するなら追加コストになる)

### Bun (`uptrace/bun`)
- 棄却理由:
  - クエリビルダ型で柔軟性は高いが、 生成型による型安全性は sqlc が上
  - 採用例が sqlc / GORM / ent と比べて少なく、 ユースケースの参考事例が少ない

## Trade-offs

良い面:
- SQL が SQL のまま書け、 schema-qualified なクエリも素直に表現できる
- 引数 / 結果が Go 型として生成され、 コンパイル時に型整合性が保証される
- migration ツール (`golang-migrate`) と DB アクセスツールが分離され、 各々がシンプルなまま
- TS 版 (Prisma / ORM) との設計差分が明確 (比較に向く)

悪い面 / コスト:
- 動的に組み立てるクエリ (検索条件が可変など) は sqlc では書きにくい。 必要なら raw query を個別に書く判断が必要
- 生成物が PR diff に混じる
- BC が増えるたびに `sqlc.yaml` に entry を足す手数 (depguard とは違いパターン化できない)
- BC 横断クエリ (Composite Read Model、 Orders の read 側 Step 6 予定) では BC 境界を跨ぐ query をどこに置くかの判断が必要 (現時点では未決定)

## When to revisit

- 動的クエリ要件が増えてきた時: 別ライブラリの併用 / 部分的に pgx 直書きの混在も検討
- BC 横断クエリ (Step 6 CQRS read) を実装する時: 配置先を別 ADR で決定
- sqlc の generate が CI 上で重くなってきた時 (BC 数が増えた場合): 並列化 / 増分生成の検討

## References

- sqlc: <https://docs.sqlc.dev/>
- sqlc + pgx/v5: <https://docs.sqlc.dev/en/stable/reference/config.html>
- pgx: <https://github.com/jackc/pgx>
- TS 版 README (Prisma 採用の背景): `../../../ec-modular-monolith-prototype-ts/README.md`
