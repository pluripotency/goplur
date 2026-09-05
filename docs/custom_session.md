# 動的セッション接続・切断制御の設計と実装仕様書

本書では、`goplur` において SSH / Telnet / ネットワーク機器（Cisco 等）への接続コマンドの柔軟な動的生成、ログイン後の対話的昇格（Cisco の特権モード等）、および環境に応じた切断シーケンス（Telnet の `Ctrl+]` 切断など）に対応するための設計方針、実装結果、および具体的な利用方法についてまとめます。

---

## 1. 背景と課題

### 1.1 接続コマンド組み立ての固定化
従来の `goplur` では、接続コマンドの組み立てが `Session` 内部にハードコードされていました：
- **SSH**: `ssh <Username>@<AccessIP>` を基本とし、ポート（`-p`）や既存の `SSHOptions` 文字列の追加のみ対応。
  - `ssh <AccessIP> -l <Username> -i ./ssh/id_rsa.key` のようなフラグ順序の変更や、鍵認証の構造的指定が不可能でした。
- **Telnet**: `telnet <AccessIP> [port]` の固定フォーマット。

### 1.2 単一文字列に依存した終了処理の限界
従来の `RunSession` における終了処理は、`currentNode.GetExitCommand()` が返す単一の文字列（デフォルト `"exit"`）を 1 回送信するのみでした。
しかし、以下のような実運用環境ではこの方式では終了できません：
- **Telnet のエスケープ切断**:
  - シェルが常駐していない機器やアプライアンスでは、`exit` コマンドを受け付けず、**エスケープ文字 `Ctrl+]`（ASCII `0x1D`）を送信して `telnet>` プロンプトに入り、`quit` を送信して切断する**必要があります。
- **Cisco 等のネットワーク機器**:
  - 特権 EXEC モード（`#` プロンプト）からログアウトするには、まず `disable`（または `exit`）で一般 EXEC モード（`>` プロンプト）に戻り、さらに `exit` を送信して切断するという多段階のシーケンスが必要です。

### 1.3 接続・昇格フローの対話性
Cisco 機器等では、ログイン成功後に `enable` コマンドを実行し、特権パスワードを入力して特権モードに昇格してから作業を行う運用が一般的です。接続処理自体を単一のコマンドライン実行だけでなく、対話的な接続ハンドラーとしてフックできる仕組みが求められていました。

---

## 2. 実装方針とアーキテクチャ

本改修では、**関心の分離（Separation of Concerns）** と **多段フォールバック設計** を採用しました。

```
【接続ライフサイクル (Connect)】
  ├─ 1. ConnectHandlerFunc (対話関数フック) が設定されている場合:
  │      最優先で実行（Cisco の enable 昇格や二段階認証を含む対話フロー）
  │
  └─ 2. 未設定の場合:
         ├─ SSHCommandProvider / TelnetCommandProvider からコマンドを取得
         │  (CommandFunc によるカスタム生成 -> 各種プロパティ組み立て -> デフォルト)
         └─ 標準ログインシーケンス (DefaultSshLogin / DefaultTelnetLogin) を実行

【終了ライフサイクル (Exit)】
  ├─ 1. ExitHandlerFunc (関数フック) が設定されている場合:
  │      最優先で実行 (Telnet の Ctrl+] -> quit や Cisco の disable -> exit)
  │
  └─ 2. 未設定の場合:
         従来の GetExitCommand() (デフォルト: "exit") を送信
```

### 主な設計ポイント

1. **`SessionExecutor` インターフェースの導入**:
   - `src/node` パッケージに、セッション操作に必要な最小限のインターフェース（`Run`, `Send`, `SendLine`, `SendControl`）を定義。
   - `src/session` との循環インポート（Circular Dependency）を防ぎつつ、ハンドラー関数内でセッション操作を型安全に行えるようにしました。

2. **制御文字マップの拡充**:
   - `Session.SendControl(char string)` を追加し、`Ctrl-]`（`\x1d`）や `Ctrl-C`（`\x03`）、`Ctrl-[`（`\x1b` = ESC）などの制御コードを即座に送信可能にしました。

3. **プリセットとビルダーパターンの提供**:
   - `SshNode` に `WithKey`, `WithLoginFlag`, `WithCommandFunc` などを追加。
   - `TelnetNode` に `WithEscapeExit()`, `WithCommandFunc` などを追加。
   - ワンライナーやメソッドチェーンで直感的に設定できるようにしました。

---

## 3. 実装結果（変更ファイル一覧）

| ファイル | 主な変更内容 |
| :--- | :--- |
| `src/node/node.go` | - `SessionExecutor` インターフェースの定義<br>- `ConnectHandlerFunc`, `ExitHandlerFunc` の型定義<br>- `Node`, `BaseNode`, `BashNode` へのハンドラー追加<br>- `SshNode` への `KeyPath`, `UseLoginFlag`, `CommandFunc`, `GetSSHCommand()` 実装<br>- `TelnetNode` への `CommandFunc`, `GetTelnetCommand()`, `TelnetEscapeExitHandler`, `WithEscapeExit()` 実装 |
| `src/session/session.go` | - `getControlChar` の拡充（`]` で `\x1d` を返却など）<br>- `Session.Send()`, `Session.SendLine()`, `Session.SendControl()` パブリックメソッド追加<br>- `DefaultSshLogin()`, `DefaultTelnetLogin()` の分離・公開<br>- `Bash()`, `Ssh()`, `Telnet()` での `ConnectHandler` 優先実行 |
| `src/session/session_wrap.go` | - `RunSession()` の開始時に `ConnectHandler` を最優先実行<br>- `RunSession()` の終了時に `ExitHandler` を最優先実行 |
| `goplur.go` | - `SessionExecutor`, `ConnectHandlerFunc`, `ExitHandlerFunc`, `TelnetEscapeExitHandler` 等の re-export |
| `src/node/node_test.go` | - コマンドビルダー、各オプション、Telnet エスケープ切断のユニットテスト追加 |
| `src/session/session_custom_test.go` | - 制御文字、カスタム ExitHandler / ConnectHandler、SendLine 連携テスト追加 |

---

## 4. 使い方と実装例

### 4.1 Telnet 接続の例

#### (1) `Ctrl+]` → `quit` [Enter] で切断する（プリセット利用）
`WithEscapeExit()` を呼ぶだけで、セッション終了時に自動的に `Ctrl+]` を送信し、`quit` を送って切断します。

```go
package main

import (
	"fmt"
	"log"

	"goplur"
)

func main() {
	// ノード作成後に WithEscapeExit() を付与
	node := goplur.NewTelnetNode("core-sw", "192.168.1.1", "admin", "password", "generic").
		WithEscapeExit()

	err := goplur.RunTelnet(node, nil, func(s *goplur.Session) error {
		out, err := s.Run("show status")
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	})
	// RunTelnet 終了時に自動で Ctrl-] -> quit が実行されて安全に切断される
	if err != nil {
		log.Fatalf("session failed: %v", err)
	}
}
```

#### (2) Telnet 切断シーケンスを完全カスタムする場合
プロンプト待ちやタイムアウトを厳密に制御したい場合は、`ExitHandler` を直接記述できます。

```go
node := goplur.NewTelnetNode("core-sw", "192.168.1.1", "admin", "password", "generic")

node.ExitHandler = func(s goplur.SessionExecutor, n goplur.Node) error {
	// 1. Ctrl-] (ASCII 0x1d) を送信
	if err := s.SendControl("]"); err != nil {
		return err
	}

	// 2. telnet> プロンプトを待って quit を送信
	sess := s.(*goplur.Session)
	rows := []goplur.ExpectRow{
		{Pattern: `telnet>`, Reaction: goplur.ReactionSendLine, Arg: "quit", Label: "quit telnet"},
		{Pattern: `Connection closed`, Reaction: goplur.ReactionSuccess, Arg: true, Label: "connection closed"},
		{Pattern: "", Reaction: goplur.ReactionSuccess, Arg: true, Label: "fallback"},
	}
	_, err := sess.Do("", rows, 3*goplur.DefaultTimeout)
	return err
}
```

#### (3) Cisco 機器への Telnet 接続（特権モード `enable` 昇格 ＋ `disable` / `exit` 切断）
ログイン後に `enable` で特権 EXEC モード（`#`）に昇格し、作業終了後に `disable` → `exit` で抜ける完全なフローです。

```go
type CiscoNode struct {
	goplur.TelnetNode
	EnablePassword string
}

func NewCiscoNode(host, ip, user, pass, enablePass string) *CiscoNode {
	n := &CiscoNode{
		TelnetNode:     *goplur.NewTelnetNode(host, ip, user, pass, "cisco"),
		EnablePassword: enablePass,
	}
	// 特権モードのプロンプトを待受プロンプトに指定
	n.WaitPrompt = fmt.Sprintf(`%s#`, host)

	// [接続ハンドラー]: Telnet ログイン後に enable コマンドを実行
	n.ConnectHandler = func(s goplur.SessionExecutor, _ goplur.Node) error {
		sess := s.(*goplur.Session)
		// 1. 標準 Telnet ログイン
		if err := sess.DefaultTelnetLogin(&n.TelnetNode); err != nil {
			return err
		}
		// 2. enable 実行とパスワード入力
		rows := []goplur.ExpectRow{
			{Pattern: `[Pp]assword:`, Reaction: goplur.ReactionSendPass, Arg: n.EnablePassword, Label: "Enable password"},
			{Pattern: n.WaitPrompt, Reaction: goplur.ReactionSuccess, Arg: true, Label: "Privileged prompt"},
		}
		_, err := sess.Do("enable", rows, 5*goplur.DefaultTimeout)
		return err
	}

	// [終了ハンドラー]: 特権モードを抜けてからログアウト
	n.ExitHandler = func(s goplur.SessionExecutor, _ goplur.Node) error {
		sess := s.(*goplur.Session)
		// 1. disable で一般 EXEC モードに戻る
		_, _ = sess.Run("disable")
		// 2. exit で Telnet セッション切断
		return sess.SendLine("exit")
	}

	return n
}
```

---

### 4.2 SSH 接続の例

#### (1) 秘密鍵と `-l` フラグを指定（ビルダーメソッド利用）
`ssh -i ./ssh/id_rsa.key 192.168.10.22 -l admin` のようなコマンドを簡単に生成できます。

```go
sshNode := goplur.NewSshNode("webserver", "192.168.10.22", "admin", "secret", "ubuntu").
	WithKey("./ssh/id_rsa.key").
	WithLoginFlag(true)

// 生成されるコマンド:
// ssh -i ./ssh/id_rsa.key 192.168.10.22 -l admin
err := goplur.RunSsh(sshNode, nil, func(s *goplur.Session) error {
	out, err := s.Run("uname -a")
	fmt.Println(out)
	return err
})
```

#### (2) `CommandFunc` で完全に自由な接続コマンドを動的生成する
ノードのプロパティ（`AccessIP` や `Username`）を参照しながら、任意の接続コマンド文字列を組み立てることができます。

```go
sshNode := goplur.NewSshNode("webserver", "192.168.10.22", "admin", "secret", "ubuntu")

// 作成後にカスタムコマンド生成関数を設定
sshNode.WithCommandFunc(func(n *goplur.SshNode) string {
	return fmt.Sprintf("ssh %s -l %s -i ./ssh/id_rsa.key", n.AccessIP, n.Username)
})

// 生成されるコマンド:
// ssh 192.168.10.22 -l admin -i ./ssh/id_rsa.key
err := goplur.RunSsh(sshNode, nil, func(s *goplur.Session) error {
	return nil
})
```

#### (3) Jump ホスト（踏み台）経由の SSH コマンド例
```go
sshNode := goplur.NewSshNode("internal-db", "10.0.1.50", "dbuser", "password", "rocky9").
	WithCommandFunc(func(n *goplur.SshNode) string {
		return fmt.Sprintf("ssh -J jumpuser@bastion.example.com %s@%s", n.Username, n.AccessIP)
	})
```

---

## 5. まとめ

1. **完全な下位互換性**: 既存の `NewSshNode`, `NewTelnetNode`, `RunSsh`, `RunTelnet` のコードは 1 行も変更することなくそのまま動作します。
2. **高拡張性**: `CommandFunc`, `ConnectHandler`, `ExitHandler` の 3 つのフックにより、将来どんな特殊な環境やネットワーク機器、認証手順が登場しても、コアライブラリに手を入れることなく呼び出し側で解決できます。
3. **実用性**: Telnet のエスケープシーケンス切断（`WithEscapeExit`）など、現場で頻出するパターンをプリセットとして提供し、利便性を向上させています。
