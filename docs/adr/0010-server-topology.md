# ADR 0010: 起動形態 (1 プロセス・1 ポート、 gRPC のみ、 Step N で REST gateway 検討)

- Status: Accepted (2026-05-17)
- 関連 ADR: 0004 (API スキーマ、 REST gateway は Step N) / 0008 (tx 伝搬、 interceptor で全 RPC に介在) / 0003 (DI 配線)
- 関連 (TS 版): TS は単一プロセスで Hono の REST、 ADR は無し (単純なため)

## Context

「内部 gRPC モノリス」 と 「公開 REST ゲートウェイ」 という 2 サーフェスの構成を採るが、 Step 0+1 では業務ロジックが薄く、 公開 REST はまだ無い。 過剰なプロセス分割は検証の見通しを下げるため、 最初は 1 プロセス・1 ポートで始める。

加えて、 「プロセスを分けると tx 共有できない」 (= BC 横断 tx が不可能になる) という Go 特有の物理制約が後の Step 5 (placeOrder オーケストレーション) に影響する。 まずは 1 プロセスで BC 横断 tx を成立させ、 REST gateway を生やす Step N でプロセス分離の是非を実地検討する。

## Decision drivers

- Step 0+1 段階で BC 横断 tx (Step 5) を成立させたい
- 「最終的に 2 プロセス (内部 gRPC / 公開 REST)」 のオプションを Step N で取れる柔軟性は残す
- プロトタイプの導入コストを最小化 (1 binary、 1 docker-compose service)
- TS 版が単一プロセスで完結している (比較を成立させるためのベースライン)

## Decision

### Step 0+1: 1 プロセス・1 ポート

- `cmd/internal-server/main.go` が唯一のエントリポイント
- 起動形態: gRPC server on `:9090` (env `GRPC_PORT` で上書き可)
- `cmd/rest-gateway/` ディレクトリは予約 (Step N で生やすが、 Step 0+1 では空 or 未作成)

### main.go の構成

```go
func main() {
  // 1. 構造化ログ (log/slog JSON handler) を default に
  // 2. envconfig で Config を load (DATABASE_URL 必須、 GRPC_PORT デフォルト 9090)
  // 3. signal.NotifyContext で SIGINT / SIGTERM を ctx に紐付け
  // 4. pgxpool.New (lazy connect)
  // 5. grpc.NewServer + UnaryTxInterceptor (ADR 0008)
  // 6. 各 BC module を Register*Server で登録 (Step 1 で members、 etc.)
  // 7. health 2 種 + reflection を登録
  // 8. SIGTERM 受信で GracefulStop
  // 9. s.Serve(lis)
}
```

### Health endpoint 2 種を提供

- **`health.v1.HealthService/Ping`** (本プロトの proto-first 自作)
  - 「proto → 生成 → server registration」 のフローが動いていることのスモークテスト
  - Step 0 で `grpcurl ... Ping` が成功すれば codegen + DI 配線が正しいことが確認できる
- **`grpc.health.v1.Health/Check`** (grpc-go 同梱、 標準)
  - k8s liveness probe / external monitoring 用
  - ops ツール (grpcurl, ghealthcheck, kubectl など) が前提とする標準サーフェス

役割が違うので両方残す。 将来 (Step N) も維持する想定。

### Reflection を常時 ON

- `reflection.Register(s)` で `grpc.reflection.v1.ServerReflection` を有効化
- 開発 / 運用での `grpcurl -plaintext ... list` を可能にする
- 本番運用時に意図的に絞る (production-only build flag 等) のは Step N 以降の課題

### Config 読み込み (envconfig)

```go
type Config struct {
  DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
  GRPCPort    string `envconfig:"GRPC_PORT" default:"9090"`
}
```

- `.env` ファイル直接読み込みはしない (env を流し込むのは docker-compose / シェル / direnv の責務)
- `.env.example` をリポジトリに同梱、 起動者は手元で `.env` を作って `direnv` / `set -a; source .env` などで流す

### Graceful shutdown

- SIGINT / SIGTERM 受信時、 `s.GracefulStop()` で in-flight RPC を完了させてから終了
- pgxpool は `defer pool.Close()` で明示クローズ

### 単一 pgxpool を共有

- pool は main で 1 個作って全 BC で共有
- ADR 0008 の interceptor が pool を保持し、 ctx に tx を載せる
- 別プロセス分離が来た時は各プロセスが個別 pool を持つ (tx 共有不可)

## Alternatives considered

### 最初から 2 プロセス (内部 gRPC + 公開 REST 別 binary)
- 棄却理由:
  - Step 0+1 では公開 REST が無いので空 binary
  - BC 横断 tx の検証 (Step 5) が先送り or 不可能になる
  - プロトタイプの導入摩擦が増える

### 公開 REST も同プロセス (Echo / Hono 相当を gRPC server と共存)
- 棄却 (一時保留):
  - Step N で必要になった時に判断
  - 「同プロセス内 / 別 listener (別ポート)」 「同プロセス・別 mux」 「別プロセス」 の 3 案を実地で比較する材料を残す

### gRPC port + grpc-web を同じポートで cmux で多重化
- 棄却理由:
  - 早すぎる最適化、 まず 1 ポートで動かす
  - cmux の運用知見コストが Step 0 の射程外

### 別途 metrics / pprof 用のポートも開ける
- 棄却 (一時保留):
  - 観測性は Step 0+1 のスコープ外 (OTel / Prometheus は後送り)
  - 入れる時は :6060 (pprof) / :9091 (Prometheus) が定番

## Trade-offs

良い面:
- 1 binary・1 ポートで Step 0+1 の検証が完結
- main.go の構造が小さく、 後で読み返しやすい
- BC 横断 tx が物理制約として可能
- REST gateway 追加時の選択肢 (同プロセス / 別プロセス) を後で取れる

悪い面 / コスト:
- 内部 / 公開のサーフェスが同じプロセスに同居する将来形を採ると、 認証・rate limit・観測性の責務が混ざる
- gRPC + REST を同居させると依存が膨らむ可能性
- production 化時に reflection を絞る作業が後追いになる

## When to revisit

- 公開 REST gateway を生やす時 (Step N): 同プロセス / 別プロセス / 別 mux の 3 案を比較する別 ADR
- 認証 / authz / rate limit を入れる時: interceptor 配線が複雑になりすぎたら別 ADR
- 観測性 (OTel / Prometheus / pprof) を入れる時
- gRPC-web / Connect ベースの混在配信を検討する時

## References

- grpc-go: <https://pkg.go.dev/google.golang.org/grpc>
- envconfig: <https://github.com/kelseyhightower/envconfig>
- log/slog: <https://pkg.go.dev/log/slog>
- standard grpc health: <https://pkg.go.dev/google.golang.org/grpc/health>
