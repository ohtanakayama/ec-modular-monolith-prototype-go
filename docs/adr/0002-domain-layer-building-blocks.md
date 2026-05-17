# ADR 0002: ドメイン層の building blocks (Entity / VO / Repository Port)

- Status: Accepted (2026-05-17)
- 関連 ADR: 0001 (モジュール境界、 Step 2 で predict) / 0003 (DI、 repository は usecase に注入) / 0007 (sqlc、 infra の repository 実装) / 0008 (tx 伝搬、 ctx 経由)
- 関連 (TS 版): [0002 Domain layer building blocks](../../../ec-modular-monolith-prototype-ts/docs/adr/0002-domain-layer-building-blocks.md)

## Context

TS 版 ADR 0002 では Entity / Value Object / Repository Port の 3 種を採用。 Go 版でも同じ構造を取りたいが、 Go の型システム特有の事情に合わせて表現方法を決める必要がある:

- Entity の不変条件をどう守るか (Go には `private` 修飾子が無く、 package 可視性で代用)
- VO を struct で表現するときの zero-value 問題 (`Email{}` がコンパイル可能、 実行時に無効)
- factory を 1 つにするか分けるか (新規作成 vs 永続化からの復元)
- repository が返すエラーの型契約

C14 で members BC の domain 層を初実装するため、 ここで方針を確定する。

## Decision drivers

- TS 版 (`class` + private + `static of/tryOf`) と意味論を揃える
- Go の package 可視性で aggregate 境界をコンパイラに強制させる
- VO の zero value が型的に許容されてしまう問題を許容しつつ、 「正しい構築経路」 を明示する
- sqlc 生成型 / pb 生成型を domain に漏らさない

## Decision

### Aggregate root は package 化、 フィールドは package-private

- 1 aggregate = 1 package (`internal/modules/<bc>/domain/<aggregate>/`)
- フィールド名は小文字 (package-private)、 getter で公開
- 集約境界外 (他 BC、 同 BC の usecase / handler / infra) から **フィールド直接アクセス不可** をコンパイラレベルで強制

```go
package member

type Member struct {
    id        string
    email     Email
    name      MemberName
    createdAt time.Time
}

func (m *Member) ID() string           { return m.id }
func (m *Member) Email() Email         { return m.email }
func (m *Member) Name() MemberName     { return m.name }
func (m *Member) CreatedAt() time.Time { return m.createdAt }
```

### VO は同 package 内に struct + 構築関数で表現

- `type Email struct { value string }` のように 1 フィールド struct
- 構築関数は `NewX(raw) (X, error)` 形式。 失敗時は `*ValidationError` (ADR 0009)
- 取り出しは `String()` または `Value()` メソッド (型に応じて)
- VO の zero value は **無効** という規約。 ただしコンパイラレベルでは防げない (Go の制約)

```go
type Email struct { value string }

func NewEmail(s string) (Email, error) {
    if !emailRe.MatchString(s) {
        return Email{}, derrors.NewValidationError("email", "invalid email format")
    }
    return Email{value: s}, nil
}

func (e Email) String() string { return e.value }
```

### Factory は 1 つに統一

- TS 版は `of`/`tryOf`/`unsafe` の 3 系統 (static methods)
- Go 版は構築コンテキスト (新規 / DB ロード) で **factory を分けない**:
  - `New(id, email, name, createdAt)` 1 つだけ
  - usecase が 「新規作成 = 自前で UUID を生成、 createdAt = now」 を組み立て、 `member.New(...)` に渡す
  - repository が DB から取得した row を VO に変換し、 `member.New(...)` に渡す
- 理由: 「VO 化を呼び出し側に集約 → factory はそれを束ねるだけ」 の構造の方が、 多重 factory より読みやすい (プロト規模)

### Repository Port (interface) を同 package に置く

- `domain/<aggregate>/repository.go` に interface 定義
- 実装は infra 側 (`internal/modules/<bc>/infra/repository.go`)、 sqlc 生成型に依存
- repository interface の **メソッド戻り値はドメインの型のみ** (sqlc model や pb 型を返さない)
- 失敗時の error type:
  - `Save(ctx, *Member) error`: 一意性違反は `*ConflictError`、 それ以外は infra エラー
  - `FindByID(ctx, id) (*Member, error)`: 該当なしは `*NotFoundError`

```go
type MemberRepository interface {
    Save(ctx context.Context, m *Member) error
    FindByID(ctx context.Context, id string) (*Member, error)
}
```

### ドメインイベント・ドメインサービスは Step 1 では入れない

- TS 版でもドメインイベント無し
- 必要になったら (Step 5+ で BC 横断の連鎖) 別 ADR

### 等価性比較 (Equals)

- Go では `==` が値同型 struct なら自然に動く (Email 同士の `==` は内部 value 比較になる)
- 明示的に `Equals(other) bool` メソッドは作らない。 必要になったら追加

### Test の位置

- 各 VO の unit test は同 package に `_test.go` で隣接 (`email_test.go`, `name_test.go`)
- package_test (external) で書く: `package member_test`
- 外部 import 経路 (`import .../domain/member`) でテストすることで、 公開 API のみ使う前提を強制

## Alternatives considered

### Entity のフィールドを大文字 (公開) にする
- 棄却理由:
  - 不変条件 (例: `email` が `*` を含むかの検査) を constructor で守る意味が無くなる
  - 他層から直接書き換え可能になり、 aggregate 境界が崩れる

### VO を type alias (`type Email = string`) で表現
- 棄却理由:
  - validation を強制する経路が無い (任意の string が代入可能)
  - 型の意味が string と同一になり、 型安全性のメリットが消える

### VO を `type Email string` (basic type 派生) で表現
- 棄却 (一時保留):
  - 「string と区別される独自型」 になる利点はある
  - 一方で `Email("invalid")` でキャストすると validation を回避できる
  - struct + 構築関数の方が 「正しい経路を 1 つにする」 意図が強い

### Factory を 2 種類 (`Register` で新規、 `Restore` で DB 復元)
- 棄却 (一時保留):
  - 意味論的には明確 (新規 / 復元の区別が型で出る)
  - ただし 「createdAt を呼び出し側が用意する」 という現状の構造でも、 「factory 2 つ」 の差分は createdAt の出所だけで本質的には同じ
  - Step 5 でドメインイベントを発火させる必要が出たら `Register` / `Restore` 分割を再検討

### `static unsafe` 相当 (validation skipping コンストラクタ) を提供
- 棄却:
  - テスト時に validation を回避したい需要があるが、 Go では `New(...)` 経由を強制し、 テスト fixture も valid な入力を作る方が混乱が少ない

## Trade-offs

良い面:
- aggregate 境界がコンパイラレベルで強制 (TS のクラス private に近い表現)
- VO の構築失敗が typed error (`*ValidationError`) として伝播し、 handler の switch がクリーン
- repository port の戻り値が domain 型のみで、 infra / pb / sqlc の漏れ出しが無い
- TS 版と意味論が対応 (比較成立)

悪い面 / コスト:
- VO の zero value (`Email{}`) が型的には許容される (実行時に無効値が紛れ込む余地)
- factory 1 種類の方針だと 「新規作成 / DB 復元」 の差が呼び出し側に出る (usecase / repository が UUID / createdAt の取り回しをする)
- Equals を明示しないため、 一部の比較で意図しない動作が出る可能性 (現時点では問題なし)
- 1 aggregate = 1 package のため、 多くの aggregate を抱える BC では package 数が増える

## When to revisit

- ドメインイベントが必要になった時 (Step 5+ で BC 横断の連鎖が増えた段階)
- VO の zero value 問題が事故を起こした時 (linter / 自前型検査で対処)
- 集約間の不変条件 (e.g., aggregate A の状態が B の状態に依存) を表現する必要が出た時
- Equals が必要になった時 (collection 操作などで)

## References

- TS 版 ADR 0002: `../../../ec-modular-monolith-prototype-ts/docs/adr/0002-domain-layer-building-blocks.md`
- Effective Go (interface / struct): <https://go.dev/doc/effective_go>
