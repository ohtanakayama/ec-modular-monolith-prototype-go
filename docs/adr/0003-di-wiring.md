# ADR 0003: DI 配線方針 (manual + 関数カリー化、 wire/fx 不採用)

- Status: Accepted (2026-05-17)
- 関連 ADR: 0008 (tx 伝搬、 ctx に乗るので DI 構造には現れない) / 0010 (起動形態、 main.go で集約) / 0002 (ドメイン層 building blocks、 Step 1 で予定)
- 関連 (TS 版): [0003 Dependency inversion and DI](../../../ec-modular-monolith-prototype-ts/docs/adr/0003-dependency-inversion-and-di.md)

## Context

各 BC は domain (port を定義) / usecase / infra (port を実装) / grpc handler の 4 層で構成する。 これらの依存関係を解決して `*grpc.Server` に乗せるまでの配線方法を決める必要がある。

Go の DI には大別して 3 系統:

1. **Manual wire (関数引数で手で渡す)**
2. **Compile-time DI**: `google/wire`
3. **Runtime DI**: `uber-go/fx`、 `samber/do`

TS 版 (ADR 0003) は manual wire + 関数カリー化を採用。 同じ意味論を Go で実現するか、 Go 文化に合った別ルートを選ぶかの判断が必要。

## Decision drivers

- BC が高々 5 つ (Members / Products / Inventory / Payments / Orders)、 DI 木は小さい
- main.go を読めば配線が一望できる方が、 プロト段階の理解コスト最小
- TS 版と意味論を揃え、 比較を成立させたい (関数カリー化 + manual wire)
- 「DI フレームワーク導入による魔術 / 暗黙挙動」 を避けたい
- Aggregate 境界の package 可視性 (ADR 0001 で予定) と相性が良いこと

## Decision

### Manual wire + 関数カリー化

- DI フレームワーク (`wire` / `fx` / `do`) は **採用しない**
- usecase は **関数カリー化** で repository を束縛する形:

```go
// internal/modules/members/usecase/register.go
type RegisterInput struct { Email, Name string }
type RegisterFunc func(context.Context, RegisterInput) (*member.Member, error)

func NewRegister(repo member.MemberRepository) RegisterFunc {
    return func(ctx context.Context, in RegisterInput) (*member.Member, error) {
        // ...
    }
}
```

- handler 層は usecase 関数を受け取る struct:

```go
// internal/modules/members/grpc/handler.go
type grpcHandler struct {
    register usecase.RegisterFunc
    findByID usecase.FindByIDFunc
}
```

### 各 BC は `module.go` で配線を集約

- パス: `internal/modules/<bc>/module.go` (Step 1 から導入)
- 公開シンボルは 2 つだけ:
  - `NewModule(pool *pgxpool.Pool) *Module` ── BC 内の repo / usecase / handler を組み立て
  - `(*Module).RegisterTo(s *grpc.Server)` ── 自 BC の gRPC service を server に登録

```go
// internal/modules/members/module.go
type Module struct {
    handler *grpcHandler
}

func NewModule(pool *pgxpool.Pool) *Module {
    repo := infra.NewMemberRepository(pool)
    return &Module{
        handler: &grpcHandler{
            register: usecase.NewRegister(repo),
            findByID: usecase.NewFindByID(repo),
        },
    }
}

func (m *Module) RegisterTo(s *grpc.Server) {
    pb.RegisterMemberServiceServer(s, m.handler)
}
```

- BC 内部の repo / usecase / handler 型は **package-private** (`type grpcHandler struct {...}` のように小文字)
- BC 外から触れるのは `Module` と `NewModule` / `RegisterTo` のみ

### main.go は 「Module 集約配線」 のみ

```go
membersMod := members.NewModule(pool)
productsMod := products.NewModule(pool)
// ...
membersMod.RegisterTo(s)
productsMod.RegisterTo(s)
```

- BC 配線の詳細は各 module.go に閉じ、 main.go は組み立ての一覧表
- 「main.go を読めば BC 構成が一望できる」 状態を維持

### Step 0 段階の状態

C08 時点では実 BC が無く、 `internal/modules/health/` のみ存在する。 health は DB 不要のため `module.go` パターンは省略し、 単に `health.NewServer()` で grpc 登録。 ADR 0003 の完全な適用は Step 1 (C18) の members module.go から開始。

### BC 間呼び出し (Step 5 の placeOrder 想定)

- BC 間で互いを呼ぶ場合、 呼ばれる側 BC が公開するのは usecase 関数の **シグネチャ型** (例: `members.FindByIDFunc`)
- 呼ぶ側 BC は `module.go` の `NewModule` で他 BC の usecase 関数を引数として受け取る
- 直接他 BC の handler / repo を参照しない (package 可視性で物理強制)
- 詳細は Step 5 で別 ADR or 本 ADR の更新

### tx 伝搬は DI に出さない

ADR 0008 の通り、 tx は ctx 経由なので関数シグネチャに現れない。 repository が `tx.GetTx(ctx)` で取り出す。 これにより usecase / repository の DI が tx の有無で揺れない。

## Alternatives considered

### `google/wire` (compile-time)
- 棄却理由:
  - 配線が `wire_gen.go` (自動生成) に分散し、 main.go から一望できない
  - BC 5 個規模では 「コード生成」 の恩恵より読みにくさのコストの方が大きい
  - エラーメッセージが分かりにくい (provider 不足など)

### `uber-go/fx`
- 棄却理由:
  - runtime DI フレームワークで lifecycle 管理も背負うが、 grpc-go の Server 起動はもう同等のパターンが手書きで書ける
  - "module" "lifecycle" の抽象が理解コスト
  - go 標準を超える依存を最小化したい (プロト段階)

### 構造体ベースの DI (`type RegisterUC struct { repo MemberRepository }`)
- 棄却理由:
  - TS 版が関数カリー化を採用、 比較軸を揃える
  - struct の method として書くと、 BC 間呼び出しで 「型シグネチャだけ公開」 (Step 5) がやりにくい (struct 型を export する必要が出る)
  - シングルトン的な usecase インスタンスを保持する意味が薄い

### main.go で全配線を直書き (module.go なし)
- 棄却理由:
  - BC 増加で main.go が肥大化
  - BC 境界 (package 可視性) を活かす土台として `module.go` が機能 (BC 内部型を package-private にしたまま、 外部公開を `Module` 経由に絞れる)

## Trade-offs

良い面:
- main.go を読めば BC 構成が一望できる
- BC 内の型を package-private に保てる (`module.go` の `NewModule` のみ公開)
- 関数カリー化により usecase が値として扱え、 BC 間呼び出しでも 「関数シグネチャ型を渡す」 だけで結合できる
- TS 版と同型 (関数カリー化) で比較成立
- DI フレームワークの魔術 / lifecycle 抽象を背負わない

悪い面 / コスト:
- BC が大量に増えると `module.go` の `NewModule` 引数が肥大する可能性 (5 BC 程度では問題なし)
- `wire` を使えばコード生成で省ける文字数が手書きになる (BC 5 個では誤差)
- 動的に provider を差し替える (e.g., 本番 / staging で別実装) ようなケースは手動で if 分岐

## When to revisit

- BC が 10+ に増えた時: `NewModule` 引数の肥大が見るに堪えなくなったら検討
- ライフサイクル管理 (起動順序、 shutdown 順序) が複雑になった時: `fx` の lifecycle を借りる選択肢
- BC 間呼び出しの実例 (Step 5 placeOrder) が出てから配線の限界を再評価
- テストで provider 差し替えのボイラーが許容しがたくなった時

## References

- Effective Go (interfaces / functions): <https://go.dev/doc/effective_go>
- TS 版 ADR 0003: `../../../ec-modular-monolith-prototype-ts/docs/adr/0003-dependency-inversion-and-di.md`
