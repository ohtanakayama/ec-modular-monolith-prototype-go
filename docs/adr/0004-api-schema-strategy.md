# ADR 0004: API スキーマ戦略 (proto-first + buf、 REST は Step N)

- Status: Accepted (2026-05-17)
- 関連 ADR: 0001 (モジュール境界、 Step 2 で予定) / 0003 (DI、 C08 で予定) / 0010 (起動形態、 C08 で予定)
- 関連 (TS 版): [0004 OAS-first API スキーマ](../../../ec-modular-monolith-prototype-ts/docs/adr/0004-oas-first-api-and-interfaces-layer.md)

## Context

本プロトタイプは「内部サービス間通信は gRPC」「公開 API (ブラウザ / webhook / SaaS) は REST」 という二層構成を採る。 Step 0+1 ではまず内部 gRPC モノリス 1 プロセス・1 ポートで構築し、 REST gateway は Step N で生やす予定。

API スキーマの「唯一のソース (single source of truth)」 をどう持つか、 そこから何を / どう生成するか、 生成物を commit するか / しないかを最初に決める必要がある。 これは TS 版で OAS-first 採用 (ADR 0004 TS) と対応する論点を Go 側で再決定する作業に相当する。

加えて、 Step 0 段階では実装すべき業務 API はまだ無いので、 動作確認用に最小限の `health.v1.HealthService` だけを 1 個用意し、 codegen が回ることを確認するのが本コミット (C04) のスコープ。

## Decision drivers

- 内部 API は gRPC のみ。 proto を契約のソースにすると HTTP/JSON 直接表現より型が強く出る
- buf は lint / breaking 検知 / 複数言語対応がワンセットで、 Go コミュニティの de facto
- REST は Step N で OpenAPI-first を採るが、 本 ADR では「翻訳層が必要になる」 ことを明示するに留め、 構造の詳細は Step N で別 ADR
- 「生成物を commit するか」 は Go コミュニティの一般慣行 (kubernetes / etcd / 多数) に従う

## Decision

### 内部 API の契約は proto-first (proto を唯一のソース)

- `proto/<bc>/v1/<service>.proto` に 1 BC = 1 ディレクトリで定義
- proto package は `<bc>.v1` (例: `members.v1`)
- 全 BC の proto は 1 つの buf workspace (`proto/buf.yaml`) で管理。 BC 間の breaking 検出を後で活かす
- proto から `protoc-gen-go` + `protoc-gen-go-grpc` で Go コードを生成し、 `gen/proto/<bc>/v1/` に出力

### ツールチェーン: buf

- `proto/buf.yaml` で lint (`STANDARD`) と breaking 検出 (`FILE`) を有効化
- `proto/buf.gen.yaml` で remote プラグイン (`buf.build/protocolbuffers/go`, `buf.build/grpc/go`) を指定し、 ローカルに protoc バイナリが無くても生成が回る
- `require_unimplemented_servers=true` を有効化 → server interface の前方互換を強制 (新規 RPC 追加時に既存 server がコンパイルエラーになる)
- `managed.enabled=true` + `go_package_prefix` で proto の `option go_package` を proto ファイルに書かなくて済むようにする

### 生成物は git に commit する

- `gen/proto/**/*.pb.go` / `*_grpc.pb.go` は **commit する**
- `.gitignore` で除外はしない
- 理由:
  - `go build ./...` が buf / protoc 無しで通る (寄稿者の足場が軽い)
  - PR レビューで proto 変更の API 影響が diff として見える
  - CI で `make buf-gen` → `git diff --exit-code` を入れれば「生成漏れ」も検出できる (Step 1 以降の CI 整備時)

### REST との関係 (Step N の前置き、 本 ADR では非確定)

将来 REST gateway を生やす際は以下を方針とする (詳細は Step N で別 ADR 起こす):
- REST 側は OAS-first (TS 版と同等の方針)、 proto は内部 API のソースで REST と独立
- proto ↔ JSON を機械翻訳する `grpc-gateway` 等は **採らない予定**: 公開 API と内部 API の関心は分離されるべきで、 翻訳層は薄い手書きが見通しが良いと仮定 (Step N で実地検証)
- REST ハンドラ層 (Step N) が gRPC client を呼ぶ構成。 公開エラー形式 (RFC 7807 / 独自 JSON) と内部エラー (gRPC status) の対応は別 ADR (0009 エラーモデルの拡張) で決定
- 公開用途別 (ブラウザ / webhook / SaaS) に gateway を分ける可能性は残す。 プロセス分離するか否かは Step N の判断

### proto の lint レベルとパッケージ命名

- `STANDARD` を有効、 `PACKAGE_VERSION_SUFFIX` のみ除外 (将来 `v1alpha` 等の柔軟性を残す)
- service / rpc / message の命名は buf STANDARD の規約に従う (`service XxxService`, `XxxRequest` / `XxxResponse`)

### C04 で投入する具体物

- `proto/buf.yaml`
- `proto/buf.gen.yaml`
- `proto/health/v1/health.proto` (動作確認用、 Step 0 で `cmd/internal-server` に組み込む)
- `gen/proto/health/v1/{health.pb.go, health_grpc.pb.go}` (生成済みコミット)

## Alternatives considered

### grpc-gateway で proto から REST を自動生成
- 棄却理由:
  - proto アノテーション (`google.api.http`) が proto 側に REST の都合を漏らす。 内部 / 公開のスキーマを混ぜると変更が連動する
  - 公開 API のエラーレスポンス形式や認証ヘッダなどの REST 固有事項を proto で表現するのは無理がある
  - 「公開 API は OAS が単一のソース」 という TS 版で得た学びと矛盾する

### Connect (`buf build connect-go`) 1 本で REST/gRPC を兼用
- 棄却理由:
  - 公開 API は SaaS 利用 / 外部 webhook を想定。 標準 REST + OAS の方が外部利用者の障壁が低い
  - 内部のみ gRPC ならグリーンフィールド grpc-go で十分

### 生成物を commit せず、 ビルド時 / postinstall で生成
- 棄却理由:
  - PR で API 変更が diff として見えない
  - 寄稿者が buf / protoc をローカル準備する必要が出る
  - Go の慣行から外れる

### `option go_package` を proto ファイルに直書きする
- 棄却理由:
  - 全 BC の proto に同じ prefix を機械的に書く負担。 `buf.gen.yaml` の `managed.enabled` で集中管理できる
  - リポジトリ移動 (org 変更等) があった時の修正箇所が増える

## Trade-offs

良い面:
- proto 1 ファイルで API 契約・型・gRPC server interface・client が全部出る
- buf lint / breaking が CI に乗せやすい
- 生成物を commit するので `go build` が外部ツール無しで通る
- REST 層を後出しで分離できる (翻訳層は薄い手書きで明示)

悪い面 / コスト:
- 生成物が PR の diff に混ざる (proto 変更時にレビューノイズ)
- proto と OAS の二重管理が Step N 以降に発生 (内部表現と外部表現が異なる前提なので、 同期はあえてしない)
- `require_unimplemented_servers=true` のため、 server に新規 RPC を実装するまでビルドが赤くなる。 「未実装まで型で気付く」 効果と引き換え

## When to revisit

- Step N で REST gateway を生やす時: 翻訳層 (proto ↔ JSON DTO) を実地で書いて、 grpc-gateway / 手書き / Connect の再評価
- 公開 API の用途別 gateway 分割 (ブラウザ / webhook / SaaS) を検討する時
- proto の breaking 検知が運用ペインになった時 (例: `FILE` ルールが厳しすぎる場合は `PACKAGE` に下げる)

## References

- buf: <https://buf.build/docs>
- protoc-gen-go-grpc unimplemented requirement: <https://pkg.go.dev/google.golang.org/grpc#section-readme>
- TS 版 ADR 0004 (OAS-first): `../../../ec-modular-monolith-prototype-ts/docs/adr/0004-oas-first-api-and-interfaces-layer.md`
