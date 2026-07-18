# Nodeインターフェース設計とリファクタリング計画 (改訂版)

`Node` を構造体からインターフェースへ移行し、特定のプロトコルや環境に特化したノード型（`BashNode`、`SshNode`、`TelnetNode`、および将来の `CiscoNode` / `CiscoConfNode`）をサポートするためのリファクタリング計画です。

---

## 1. ゴールと設計方針

### 修正内容と方針
1. **`Node` のインターフェース化**:
   `Node` を Go の `interface` に変更し、共通のゲッターを定義します。
   - `SetExitCommand` はノード生成後の変更が発生しないため、インターフェースから**除外**します。
2. **`ExitCommand` のデフォルト値（"exit"）化**:
   - `GetExitCommand` が呼ばれた際、構造体初期化時に特に指定がなく空文字列であった場合は、自動的に `"exit"` を返すように実装します。
   - これに伴い、既存の `session.go` 内の各メソッドに点在していた `node.ExitCommand = "exit"` の記述は**削除**し、Node初期化時に設定、あるいはゲッターでのデフォルト値返却に一元化します。
3. **`TelnetNode` への `TelnetPort` の追加**:
   - `TelnetNode` に `TelnetPort int` を追加し、デフォルト値を `23` に設定します。
   - `s.Telnet()` メソッドで、ポートが 23 以外の場合は `telnet <ip> <port>` となるよう引数を追加します。

---

## 2. インターフェースと各ノードの設計

### Node インターフェース
```go
type Node interface {
	GetHostname() string
	GetUsername() string
	GetPassword() string
	GetPlatform() string
	GetWaitPrompt() string
	GetAccessIP() string
	GetExitCommand() string
	GetRootPassword() string
}
```

### BaseNode 構造体 (埋め込み用)
```go
type BaseNode struct {
	Hostname     string `json:"hostname"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Platform     string `json:"platform"`
	WaitPrompt   string `json:"waitprompt"`
	AccessIP     string `json:"access_ip"`
	ExitCommand  string `json:"exit_command"`
	RootPassword string `json:"root_password"`
}

// ゲッターの実装
func (n *BaseNode) GetHostname() string { return n.Hostname }
func (n *BaseNode) GetUsername() string { return n.Username }
func (n *BaseNode) GetPassword() string { return n.Password }
func (n *BaseNode) GetPlatform() string { return n.Platform }
func (n *BaseNode) GetWaitPrompt() string { return n.WaitPrompt }
func (n *BaseNode) GetAccessIP() string   { return n.AccessIP }
func (n *BaseNode) GetRootPassword() string { return n.RootPassword }

func (n *BaseNode) GetExitCommand() string {
	if n.ExitCommand == "" {
		return "exit"
	}
	return n.ExitCommand
}
```

### BashNode 構造体
```go
type BashNode struct {
	Hostname    string `json:"hostname"`
	Username    string `json:"username"`
	Platform    string `json:"platform"`
	WaitPrompt  string `json:"waitprompt"`
	ExitCommand string `json:"exit_command"`
}

func (n *BashNode) GetHostname() string { return n.Hostname }
func (n *BashNode) GetUsername() string { return n.Username }
func (n *BashNode) GetPassword() string { return "" }
func (n *BashNode) GetPlatform() string { return n.Platform }
func (n *BashNode) GetWaitPrompt() string { return n.WaitPrompt }
func (n *BashNode) GetAccessIP() string   { return "" }
func (n *BashNode) GetRootPassword() string { return "" }

func (n *BashNode) GetExitCommand() string {
	if n.ExitCommand == "" {
		return "exit"
	}
	return n.ExitCommand
}
```

### TelnetNode 構造体
```go
type TelnetNode struct {
	BaseNode
	TelnetPort int `json:"telnet_port"`
}

func (n *TelnetNode) GetTelnetPort() int { return n.TelnetPort }
```

### SshNode 構造体
```go
type SshNode struct {
	BaseNode
	SSHPort    int    `json:"ssh_port"`
	SSHOptions string `json:"ssh_options"`
}

func (n *SshNode) GetSSHPort() int       { return n.SSHPort }
func (n *SshNode) GetSSHOptions() string { return n.SSHOptions }
```

---

## 3. 既存コードの変更範囲

### 1. `node.go`
- `Node` インターフェースを定義。
- 各種具象ノード (`BaseNode`, `BashNode`, `TelnetNode`, `SshNode`) を定義し、対応するゲッターを実装。
- コンストラクタ `NewMeNode() *BashNode` は初期状態で `ExitCommand: "exit"` を設定。
- コンストラクタ `NewSshNode(...) *SshNode` は `ExitCommand: "exit"`, `SSHPort: 22` を設定。
- コンストラクタ `NewTelnetNode(...) *TelnetNode` を追加し、`ExitCommand: "exit"`, `TelnetPort: 23` を設定。

### 2. `session.go`
- `Session` の `nodes` フィールドを `[]Node` に変更。
- 以下のメソッドから `node.ExitCommand = "exit"` の代入処理を**削除**：
  - `Bash()` メソッド内
  - `Ssh()` メソッド内
  - `Telnet()` メソッド内
- `s.Telnet()` メソッド内で、ポートがデフォルト（23）以外の場合に引数を追加する処理を実装：
```go
action := fmt.Sprintf("telnet %s", accessTarget)
if telnetNode, ok := node.(interface {
	GetTelnetPort() int
}); ok {
	tPort := telnetNode.GetTelnetPort()
	if tPort != 0 && tPort != 23 {
		action += fmt.Sprintf(" %d", tPort)
	}
}
```
- `Su` / `SudoI` メソッド内で作成する一時ノードを `&BaseNode{...}` で初期化し、`ExitCommand: "exit"` も設定。

### 3. `session_wrap.go`
- `currentNode.ExitCommand` アクセス部分をゲッター経由に書き換え：
```go
currentNode := s.CurrentNode()
exitCmd := currentNode.GetExitCommand()
```

### 4. その他ファイル
- `shell.go`、`logger.go` 内の `node.FieldName` へのアクセスを、すべてゲッターメソッド（例: `node.GetWaitPrompt()`）へ置き換え。

---

## 4. 検証計画

### 自動テスト
```bash
go test -v ./...
```
既存のテスト（Bashセッション、ファイル操作、ラッパー）が問題なく動くことを確認します。
また、新規作成した `NewTelnetNode` やポート指定が正しく機能するかどうかの検証コードも追加します。

---

## 5. リファクタリング完了のまとめ（Walkthrough）

Node構造体をインターフェースへ移行し、`SshNode`、`TelnetNode`、`BashNode` への多態性（Polymorphism）を持たせるリファクタリングを完了しました。

### 実施した変更内容

#### 1. `Node` インターフェースの定義と各種具象ノードの実装 ([node.go](file:///home/worker/Documents/antigravity/goplur/node.go))
- `Node` をゲッターを提供するインターフェースとして再定義しました。
- 各種具象ノードとして `BaseNode`、`BashNode`、`TelnetNode`、`SshNode` を実装しました。
- `TelnetNode` に `TelnetPort`（デフォルト 23）、`SshNode` に `SSHPort`（デフォルト 22）と `SSHOptions` を追加しました。
- 生成時にデフォルトで `ExitCommand = "exit"` となるようコンストラクタ（`NewMeNode`, `NewSshNode`, `NewTelnetNode`）を設定し、空文字列の場合は `GetExitCommand()` が `"exit"` をフォールバックして返すようにしました。

#### 2. セッションおよびヘルパーメソッドの更新
- **`session.go` ([session.go](file:///home/worker/Documents/antigravity/goplur/session.go))**:
  - 各種ログイン処理 (`Bash`, `Ssh`, `Telnet`) から点在していた `node.ExitCommand = "exit"` の代入を排除しました。
  - 直接のフィールド参照 (`node.WaitPrompt` 等) を `node.GetWaitPrompt()` 等のゲッター呼び出しへ変更しました。
  - `Ssh()` および `Telnet()` メソッドにおいて、具象型に応じたポート指定等の処理を型アサーションを用いて実装しました。
- **`session_wrap.go` ([session_wrap.go](file:///home/worker/Documents/antigravity/goplur/session_wrap.go))**:
  - セッションラッパー関数のシグネチャを `Node` インターフェースに変更し、`currentNode.GetExitCommand()` や `currentNode.GetUsername()` などを利用するようにしました。
- **`shell.go` ([shell.go](file:///home/worker/Documents/antigravity/goplur/shell.go))** & **`logger.go` ([logger.go](file:///home/worker/Documents/antigravity/goplur/logger.go))**:
  - `WaitPrompt` や各種ログ情報の出力をゲッターメソッド呼び出しに変更しました。

#### 3. テストの追加と既存テストの検証 ([goplur_test.go](file:///home/worker/Documents/antigravity/goplur/goplur_test.go))
- `TestNewNodeTypes` を新規追加し、新しく実装された `SshNode`、`TelnetNode`、`BashNode` の生成とゲッター/セッター、ポート指定の挙動をカバーしました。

---

### 検証結果

ローカル環境にて `go test -v ./...` を実行し、すべてのユニットテストが正常にパスすることを確認しました。

#### テスト実行ログ
```
=== RUN   TestNode
    goplur_test.go:17: Hostname: resolute, Username: worker, Platform: ubuntu resolute, WaitPrompt: worker@resolute..+\$ 
--- PASS: TestNode (0.00s)
=== RUN   TestBashSession
--- PASS: TestBashSession (0.19s)
=== RUN   TestShellOperations
--- PASS: TestShellOperations (0.16s)
=== RUN   TestSessionWrap
--- PASS: TestSessionWrap (2.16s)
=== RUN   TestNewNodeTypes
--- PASS: TestNewNodeTypes (0.00s)
PASS
ok  	goplur	2.519s
```
