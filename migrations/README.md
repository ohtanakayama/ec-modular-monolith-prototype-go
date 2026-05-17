# migrations

`golang-migrate` 形式の SQL マイグレーション。 ファイル名規約: `NNNN_<bc>__<object>.{up,down}.sql`。

## 番号の割り当て

ADR 0006 で確定した schema 分離方針に基づき、 BC ごとに番号レンジを切る。

| レンジ | BC | 用途 |
|---|---|---|
| 0000-0009 | (共通) | プレースホルダ / 拡張機能 (uuid-ossp 等) |
| 0010-0019 | members | `members` schema + テーブル |
| 0020-0029 | products | `products` schema + テーブル |
| 0030-0039 | inventory | `inventory` schema + テーブル |
| 0040-0049 | payments | `payments` schema + テーブル |
| 0050-0059 | orders | `orders` schema + テーブル |
| 9000+ | indexes / view 等の横断的な追加 | (将来) |

レンジ内に余裕があるので、 BC の追加マイグレーションは番号を増やしながら追記する。

## 実行

```bash
make migrate-up        # 全部適用
make migrate-down      # 1 つ巻き戻し
make migrate-status    # 現在のバージョン
```

## 命名規則

- `0011_create_members__member.up.sql` のように `<bc>__<table>` 区切りでテーブルを明示
- schema 作成は `0010_create_schema_members.up.sql` のように分離 (テーブル作成より前)
- cross-BC FK は張らない (ADR 0006)
- schema-qualified なクエリは sqlc 入力 SQL 側で記述
