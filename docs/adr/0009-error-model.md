# ADR 0009: エラーモデル (typed error 4 種 + gRPC Status mapping)

- Status: Accepted (2026-05-17)
- 関連 ADR: 0004 (API スキーマ、 REST 形式の確定は Step N) / 0008 (tx 伝搬、 失敗時の rollback) / 0003 (DI 配線、 mapping をどこに置くか)
- 関連 (TS 版): TS は class-based 例外 4 種 + Hono の `onError` で集約。 同じ意図を Go の typed error で実装

## Context

各 BC は domain / usecase / infra の 3 層を持ち、 失敗の意味合いはバラバラ。 「不正入力」 「見つからない」 「重複している」 「業務ルール違反」 を **transport から見て同じ扱い** にすると 422 / 500 が混在して使えないエラー応答になる。 一方、 各 BC で毎回 transport コードを返すと domain に gRPC / HTTP の知識が漏れる。

そこで:
- **domain / usecase** は transport に依存しない形でエラーの 「種類」 を表現する
- **handler (gRPC)** がそれを transport コード (gRPC Status) に変換する
- **handler (REST、 Step N)** が同じ typed error を RFC 7807 / 独自 JSON に変換する (詳細は Step N で別 ADR)

エラーの 「種類」 を表現する手段として Go では以下の選択肢:

1. sentinel error (`var ErrNotFound = errors.New(...)`)
2. typed error (struct + `Error()` メソッド)
3. error code (`type ErrorCode int` + 単一エラー型)

## Decision drivers

- 失敗の文脈情報 (どのフィールド、 どのリソース ID) を構造化して渡したい → sentinel では弱い
- `errors.As` で型ベースに分岐できる方が handler 側の switch が静的に検査される
- TS 版で class 4 種を採用しており、 構造的対応関係を取りやすい
- Go 標準のエラー wrapping (`fmt.Errorf("...: %w", err)`) との相性

## Decision

### typed error 4 種を `internal/shared/errors` に置く

| 型 | 用途 | 主なフィールド |
|---|---|---|
| `*ValidationError` | 入力検証失敗 (VO の構築失敗、 usecase 前提違反) | `Field`, `Message` |
| `*NotFoundError` | 識別子参照の miss | `Resource`, `ID` |
| `*ConflictError` | 一意性 / 識別衝突 (重複 email など) | `Resource`, `Message` |
| `*DomainError` | 業務ルール違反 (在庫不足、 状態遷移不可など) | `Code`, `Message` |

- いずれも **pointer receiver** で `Error() string` を実装し、 `errors.As(&v)` で取り出せるようにする
- コンストラクタ (`NewValidationError(field, msg) *ValidationError` 等) を提供し、 ポインタを直接組まずに済むようにする
- 4 種すべて `internal/shared/errors/errors.go` に集約 (BC 横断で同じ型を共有)

### transport mapping (gRPC) は handler 層で `errors.As`

Step 1 (C17) で各 BC の gRPC handler に mapping を導入する想定。 共通化が必要になったら `internal/shared/errors/grpc.go` 等に切り出すが、 現時点では各 handler に閉じる方針。

| ドメインエラー | gRPC Status | HTTP (Step N、 暫定) |
|---|---|---|
| `*ValidationError` | `InvalidArgument` | 400 |
| `*NotFoundError` | `NotFound` | 404 |
| `*ConflictError` | `AlreadyExists` | 409 |
| `*DomainError` | `FailedPrecondition` | 422 |
| (unwrap 不能) | `Internal` | 500 |

ペイロード (詳細 message / field 名) を gRPC trailer / status detail に載せるか、 message 文字列だけに留めるかは Step 1 (C17) の実装時に決定。 暫定では `status.Errorf` の message に詰める方針。

### エラー wrap は `fmt.Errorf("context: %w", err)`

- usecase が repository / VO の作るエラーを呼び出し元に伝える際は **そのまま返す か `%w` で wrap**
- `%w` を使うことで `errors.As` で深く wrap されたエラーも取り出せる
- 独自の `Wrap()` ヘルパーは作らない (`fmt.Errorf` で十分)

### domain / usecase は transport を知らない

- `domain/*.go` / `usecase/*.go` 内で `status.Errorf` や HTTP status を直接書かない
- ここで使ってよいのは 4 種の typed error のみ
- gRPC への変換は **handler 層** にしか書かない (DI / interceptor で集約する場合も handler 配下)

### Validation の責務配置

- VO (`NewEmail`, `NewMemberName` 等) のコンストラクタが `*ValidationError` を返す
- usecase の前提違反 (必須パラメータ欠落など) も `*ValidationError`
- gRPC proto レベルの型不一致 (フィールド型違いなど) は proto レイヤで弾かれるため、 ここに来る前にエラーになる

### Step 0 で投入する具体物 (C06)

- `internal/shared/errors/errors.go`: 4 種の typed error + コンストラクタ
- `internal/shared/errors/errors_test.go`: `errors.As` での識別、 wrap 経由の取り出し、 型同士の非マッチを検証

## Alternatives considered

### sentinel errors (`var ErrNotFound = errors.New(...)`)
- 棄却理由: 文脈情報 (どのリソース、 どの ID か) を持てない。 `errors.Is` の単純比較しかできず、 詳細を付けたい時に独自構造を被せる必要がある (結局 typed error と等価になる)

### 単一エラー型 + error code (`type Error struct { Code Code; Message string }`)
- 棄却理由: handler 側で `switch err.Code` になり、 静的検査が効きにくい (新しいコード追加時に case 漏れがコンパイル時に出ない)。 typed error なら `errors.As(&specificType)` で意図が明示される

### gRPC status をそのまま domain にも持たせる (`status.Errorf(codes.NotFound, ...)`)
- 棄却理由:
  - domain が transport 層に依存する
  - REST handler (Step N) で gRPC code → HTTP の二段変換が必要になり混乱
  - TS 版で domain が HTTP status を返さない設計を採っていることと整合しない

### 標準ライブラリ `errors.Join` で複合
- 棄却 (補足): multiple field validation エラーを返したい時には併用しうるが、 ベースの typed error 4 種とは独立した拡張で、 必要になった時点で追加

## Trade-offs

良い面:
- ドメインの失敗が transport から完全に独立する
- `errors.As` で各 transport が自前に mapping できる (gRPC は Step 1、 REST は Step N)
- VO の構築失敗を `*ValidationError` で即時返せるため、 usecase が空 transport 化される
- TS 版 (class 4 種) と意味論が一対一対応し、 比較が成立

悪い面 / コスト:
- handler ごとに mapping コードを書く (Step 1 で実装、 共通化する判断は後でも可能)
- gRPC status の detail (`google.rpc.BadRequest` 等) に構造化情報を載せたい場合、 追加実装が必要
- TS の onError 集約に比べると、 各 BC の handler に分散する見た目になる (`internal/modules/<bc>/grpc/errors.go` で局所化はする)

## When to revisit

- gRPC status detail (構造化エラー詳細) が必要になった時
- 公開 REST handler を生やす時 (Step N): RFC 7807 / 独自 JSON のどちらを採るか、 そこで本 ADR を更新 or 別 ADR (0009-rest 等)
- BC 数が増えて mapping の重複が許容しがたくなった時: `internal/shared/errors/grpc.go` 等に共通化を検討
- gRPC interceptor で例外的に typed error 以外 (panic recover など) を Internal に変換する必要が出た時

## References

- Go errors の wrap と `errors.As`: <https://pkg.go.dev/errors>
- gRPC status: <https://pkg.go.dev/google.golang.org/grpc/status>
- TS 版 README (class-based 例外 4 種): `../../../ec-modular-monolith-prototype-ts/README.md`
