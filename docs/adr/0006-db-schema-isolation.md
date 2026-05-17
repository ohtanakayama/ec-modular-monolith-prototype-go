# ADR 0006: DB は BC ごとに PG schema を分離 (B2)

- Status: Accepted (2026-05-17)
- 関連 ADR: 0007 (sqlc、 schema-qualified クエリと相性) / 0008 (tx 伝搬、 同 DB なので tx 共有可能) / 0001 (モジュール境界、 Step 2 で予定)
- 関連 (TS 版): TS は MySQL 単一スキーマ (= B1)。 本 ADR は 「Go 版は B2」 を確定して比較軸を作る

## Context

本プロトタイプの検証問いの 1 つに 「モジュール単位の DB 分離レベル」 がある。 候補:

- **B1**: 単一 DB / 単一 schema、 全 table をフラットに置く (TS 版採用)
- **B2**: 単一 DB / 複数 schema、 BC ごとに schema を切る (本 ADR 採用)
- **B3**: 完全 DB 分離 (BC ごとに別 DB / 別接続)
- (Postgres でない場合は database / catalog の概念で対応)

B3 は 「tx 共有不可」 「DB 数だけ運用負荷」 「接続文字列管理が膨らむ」 が明白で本プロト範囲外。 B1 と B2 の差を実地で測るのが本決定の眼目。

## Decision drivers

- TS 版 (B1、 MySQL) と意図的に対比して 「B2 がどんな運用負荷を生むか」 「クエリの見た目がどう変わるか」 を体感
- BC 横断 tx (Step 5 placeOrder) を成立させたい → 同 DB は必須
- BC 境界が SQL 層でも視覚的に分かること (`SELECT * FROM members.member`)
- 将来 BC を分離 (別 DB / 別サービス化) する余地を残す

## Decision

### Postgres schema を BC ごとに切る

```
ec_modular (1 database)
├── members.member
├── products.product
├── inventory.stock
├── inventory.reservation
├── payments.payment
└── orders.order
   └── orders.order_item
```

- migration が `CREATE SCHEMA IF NOT EXISTS <bc>;` でスキーマ作成
- 全テーブルは BC schema 配下 (`members.member` のように schema-qualified)
- migration は `migrations/` に全 BC まとめて配置、 ファイル名で BC を識別 (`0010_create_schema_members.up.sql`、 `0011_create_members__member.up.sql`)

### クエリは schema-qualified に書く

- sqlc の query SQL は **必ず schema 名を明示** (`SELECT * FROM members.member WHERE id = $1`)
- `search_path` に依存しない (`search_path` トリックは使わない)
- 理由:
  - search_path はセッション局所で、 connection pool の動的取得時にバグの温床
  - schema 名を明示するとクエリだけ見れば BC が分かる (BC 境界の可視化)
  - sqlc の生成型が schema 区別をできない場合でも、 SQL レベルで意図が明確

### cross-BC FK は張らない

- `orders.order.member_id REFERENCES members.member(id)` のような FK は **作らない**
- BC 境界での参照整合性は **application 層で保証** (Members.findById が成功するか確認)
- 理由:
  - cross-BC FK は将来の BC 分離 (別 DB 化) を物理的に塞ぐ
  - TS 版でも同じ方針 (B1 でも論理的に cross-BC FK を作らないルール)
  - 削除 cascade の挙動が BC 境界を越えるのは 「意図しない結合」 を生む

### schema 跨ぎ JOIN は Composite Read Model でのみ許容

- 通常の usecase / repository では schema を跨ぐクエリを書かない
- 唯一の例外: **Orders BC の CQRS read 側** (Step 6 予定)
  - 「注文一覧 + 商品情報 + 会員情報」 を 1 クエリで取得するため、 read 専用クエリで cross-schema JOIN を許可
  - 詳細は Step 6 で別 ADR (TS 版 ADR 0006 に相当)
- write 側は引き続き BC 境界を遵守

### 同 DB なので tx は全 BC で共有可能

- ADR 0008 の interceptor が始める tx は単一 connection に紐付き、 全 BC の schema にまたがって 1 つの tx で write 可能
- Step 5 の `placeOrder` で members / inventory / payments / orders に schema-qualified に write しても、 1 tx で整合性を保てる

### migrations の構造

```
migrations/
├── 0000_init.up.sql                       # 共通 (extension など)、 現状 空
├── 0000_init.down.sql
├── 0010_create_schema_members.up.sql      # Step 1
├── 0011_create_members__member.up.sql
├── 0020_create_schema_products.up.sql     # Step 2
├── 0021_create_products__product.up.sql
└── ...
```

- 番号体系: BC ごとに 10 番台を確保 (`00XX` = 共通、 `0010-0019` = members、 `0020-0029` = products、 ...)
- 1 migration = 1 SQL ファイル単位、 巨大化を避ける
- ファイル名は `<番号>_<BC>__<対象>.up.sql` 形式で BC が読み取れるように

### Step 0 段階の状態

C03 時点で `migrations/0000_init.up.sql` (空 `SELECT 1;`) のみ存在。 BC 用の schema 作成 migration は Step 1 (C12) で投入。 本 ADR は方針を確定するもので、 実 migration は Step 1 から積まれていく。

## Alternatives considered

### B1: 単一 schema (`public` または `app`)
- 棄却理由:
  - TS 版が採用しているので 「比較軸」 として面白くない
  - BC 境界が SQL 上で見えない (table 名 prefix で人間規約に依存)
  - 命名規約のドリフト (`member_*` / `members_*` / `mem_*` 混在) を防ぎにくい

### B3: 完全 DB 分離 (BC ごとに別 DB)
- 棄却理由:
  - tx 共有 (Step 5 placeOrder) が物理的に不可能 → サガパターン等の分散 tx が必要
  - 接続文字列 / pool / migration ツールの BC ごと多重化
  - 本プロトのスコープ外、 production 化検討時 (Step N 以降) に別 ADR

### `search_path` 依存で schema 名を省略
- 棄却理由:
  - pool から借りた connection の search_path 状態に依存し、 「動いてたものが pool reset で動かなくなる」 等のバグの温床
  - クエリだけ読んで BC が分からない (可視性低下)
  - sqlc / migration 側で search_path 設定の整合性を担保する手間

### BC ごとに別 user / role で schema 権限を絞る
- 棄却 (一時保留):
  - 「権限による境界強制」 は強力だが、 プロト段階では運用負荷が見合わない
  - 必要になった時 (本番化、 マルチテナント等) に別 ADR

### Cross-BC FK を張って整合性を DB に任せる
- 棄却理由:
  - BC 境界が DB レイヤで結合し、 将来分離が不可能になる
  - Cascade 削除 / 更新が BC 境界を超えて副作用化する
  - 「論理参照」 の方針が TS 版 と一致

## Trade-offs

良い面:
- BC 境界が SQL 上で可視 (`schema.table`)
- BC 横断 tx は同 DB のため引き続き可能
- 将来 BC 分離 (別 DB 化) する時の物理的障壁が低い (FK が無いので)
- migrations / sqlc / repository すべての層で BC 境界が一貫

悪い面 / コスト:
- schema 数だけ migration が増える (運用コスト微増)
- クエリに schema 名を毎回書く (`members.member` の冗長)
- Composite Read Model (Step 6) で 「どこに置くか」 の判断が必要 (write の BC 境界と 読み の BC 境界が一致しない)
- 検索系 (横断検索) や admin ツールで schema 跨ぎが頻発するなら、 都度 「これは read 側か write 側か」 の判断コスト

## When to revisit

- Step 6 (Orders CQRS read) で Composite Read Model を実装する時: 配置先の別 ADR
- 検索系 / admin 系の機能で schema 跨ぎが日常化した時
- 本番運用で 「BC 分離 (別 DB 化)」 が必要になった時: B3 への移行を別 ADR
- TS 版 (B1) との運用負荷比較が出揃った時: 本 ADR で結果を追記

## References

- TS 版 README (MySQL 単一スキーマの背景): `../../../ec-modular-monolith-prototype-ts/README.md`
- Postgres schemas: <https://www.postgresql.org/docs/16/ddl-schemas.html>
- sqlc + Postgres schema: <https://docs.sqlc.dev/en/stable/howto/select.html>
