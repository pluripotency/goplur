# ダイレクト対話モード（DirectMode）の技術背景と実装仕様書

本書では、`goplur` において接続先の OS や機器ごとのプロンプト（`WaitPrompt`）を事前に定義・待機することなく、FortiGate などのネットワーク機器や ATEN などのアプライアンス機器へ透過的に SSH / Telnet 接続し、即座にユーザー対話端末（`Interact`）へ制御を引き渡すための「ダイレクト対話モード（DirectMode）」の技術背景、アーキテクチャ設計、実装内容、および具体的な利用方法について解説します。

---

## 1. 背景と課題

### 1.1 従来の goplur の設計思想とその限界
`goplur` は、もともと「Linux サーバー上での構成自動化やスクリプトによるコマンド実行（`Session.Run()` や `Session.SedReplace()` 等）」を主たるユースケースとして設計されていました。

この自動実行モデルでは、プログラムが送信したコマンドが「いつ完了してプロンプトに戻ったか」を正確に把握する必要があるため、ノードごとにシェルの待受プロンプト（`WaitPrompt`、正規表現）を登録し、コマンド送信のたびにそのプロンプトに合致するまで出力を監視するシーケンスが基本構造となっていました。

しかし、`host-credential-manager-go`（`hcm-client`）のような **「ホスト一覧からターゲットを選択し、認証情報を取得して対話端末（PTY）を開く」** という対話型接続クライアント（Bastion / QuickConnect Client）で利用した場合、この自動化前提の仕様が大きな障害となっていました。

### 1.2 直面した具体的な問題

1. **OS / 機器ごとのプロンプト差異とタイムアウト**:
   - **Linux**: デフォルトでは `[user@host ~]$ ` や `root@host:~# ` といった正規表現でプロンプトを待機。
   - **FortiGate**: プロンプトは `hostname #` や `hostname $`、VDOM 環境では `hostname (global) #` となり、Linux の正規表現と一致せず、セッションが最大 600 秒間タイムアウトするまでブロックする。
   - **ATEN（IP-KVM / PDU / シリアルコンソール等）**: ログイン直後にメニュー選択画面（`[Menu] 1. Console ... Please type a number`）が表示されたり、そもそも伝統的なシェルプロンプト記号が存在しない。プロンプト待ちを行うと 100% タイムアウトする。
   - **Cisco / Juniper 等**: 一般 EXEC モード（`>`）や特権 EXEC モード（`#`）など、機器ごとに異なる。

2. **接続初期の遅延・DNS 逆引き遅延（UseDNS 問題）**:
   - SSH サーバー側で `UseDNS yes` が有効になっている環境や、WAN / VPN 経由の低速環境では、TCP 接続開始から最初のプロンプトが出力されるまでに 2〜5 秒以上の無音（出力が 1 バイトも届かない）期間が発生することが日常的にあります。
   - 単純に「2秒間パスワードプロンプトが来なければ鍵認証（パスワード不要）とみなす」というタイムアウト設計にすると、**まだホストが接続処理中であるにもかかわらず鍵認証成功と誤認して対話モードに入ってしまい、ユーザーが操作を始めた直後に遅れてパスワードプロンプトが降ってきて画面が崩れる** という深刻な誤検知が発生します。

3. **Linux 自動化用コマンドの副作用**:
   - `DefaultSshLogin` 完了後に `platformRun()` が呼び出され、`export LANG=C PROMPT_COMMAND=""` や `stty -echo` が自動送信される。
   - `s.Interact()` 実行時にデフォルトで `stty echo; stty sane` が送信される。
   - セッション終了時に `exit` コマンドが送信される。
   - これらは FortiGate や ATEN などのネットワーク機器・アプライアンスにおいて、構文エラーや意図しない切断・画面乱れの原因となります。

---

## 2. 設計方針とアーキテクチャ

### 2.1 本質的な発想の転換（関心の分離）
対話端末クライアント（`hcm-client` 等）が求めているのは、**「認証までを自動で行い、完了したら直ちに端末操作（PTY 入出力）を人間に引き渡すこと」** です。

ひとたび `s.Interact()`（PTY 直通モード）に突入すれば、リモートホストのすべての出力はユーザーのローカル端末画面に直接描画され、ユーザーのキーボード入力はそのままリモートホストへ送信されます。

したがって、**対話セッションにおいては、リモートホストのプロンプト文字列をプログラムが解釈・同期する必要性はゼロ（100% 不要）** です。必要なのは「認証が完了した（あるいは画面描画が始まった）」という境界線だけを安全に検知することです。

### 2.2 多段階インターバル出力判定（Multi-Stage Interval Output Detection）
遅延ホストでの誤検知を防ぎつつ、OS 非依存の高速接続を実現するため、**「Fast-Path（即時記号検知）＋ 多段階インターバル出力判定（2秒×最大3回リトライ）」** を組み合わせたハイブリッド・ステートマシンを構築しました。

```
                              ┌─────────────────────────┐
                              │  SSH / Telnet プロセス起動 │
                              └────────────┬────────────┘
                                           │
                           ┌───────────────┴───────────────┐
                           │   インターバル監視 (2.0秒単位)   │◄──────────────┐
                           └───────────────┬───────────────┘               │
                                           │                               │
        ┌───────────────────┬──────────────┴─────┬──────────────────┐      │
        ▼                   ▼                    ▼                  ▼      │
  [yes/no確認]         [Password:要求]       [エラー終了]        [2秒満了]   │
        │                   │                    │                  │      │
   "yes" 送信         パスワード送信         エラー返却       【出力検査】  │
        │                   │                                       │      │
        └───────► (リセット) └────────► 【後続出力監視へ】           │      │
                                                                    │      │
                         ┌──────────────────────────────────────────┴───┐  │
                         ▼                                              ▼  │
               [出力なし (0バイト/空白)]                    [何かしら出力あり] │
                         │                                              │  │
                         ├─ attempt < 3: もう2秒待つ ───────────────────┼──┘
                         │                                              │
                         └─ attempt >= 3: 接続タイムアウトエラー        ▼
                                                        ┌───────────────────────────────┐
                                                        │       鍵認証 / メニュー完了     │
                                                        │  WaitPrompt不要で直ちにInteract │
                                                        └───────────────────────────────┘
```

#### 判定ロジックの詳細：

1. **Fast-Path（即時検知）**:
   - 汎用プロンプト正規表現 `(?m)[\$#>:%][ \t]*$` を監視。
   - FortiGate の `#` や Linux の `$`, `#`、Cisco の `>` など、末尾記号が届いた瞬間に 2 秒の満了を待たずに **0 秒で即座にログイン完了** と判定（体感速度を最大化）。

2. **2秒インターバル満了時の出力有無判定（誤検知防止）**:
   - 2 秒経過時、累積出力文字列をトリム検査（`strings.TrimSpace(accumulatedOutput)`）。
   - **完全な無音（0バイト）の場合**:
     - まだホストが DNS 逆引き中、または暗号ハンドシェイク中と判断。
     - 決して鍵認証完了と誤認せず、リトライカウントを増やして **「もう2秒待つ」** を最大 3 回（計 8 秒）繰り返す。
   - **何かしら出力がある場合**:
     - パスワードプロンプト（`Password:`）にもエラーにも合致せず、かつテキスト（MOTD、バナー、ATEN のメニュー等）が存在する。
     - 「パスワード入力を求められることなく、鍵認証や自動認証によって画面が出力された」と確信を持って判定し、直ちに `Interact()` へ移行。

3. **パスワード送信後の新規出力判定**:
   - パスワードプロンプト検知後、パスワードを送信して直ちに「パスワード送信後モード」へ移行。
   - 送信時点の累積出力長を記録し、送信後の **新規出力（`newOutput`）** を監視。
   - 認証検証中の無音時は 2 秒待機をリトライし、新規出力（認証成功によるプロンプトやバナー）が届いた瞬間に、WaitPrompt 待ちをバイパスして直ちに `Interact()` へ移行。

4. **副作用の完全排除**:
   - `DirectMode` 有効時は、`platformRun()`（`export LANG=C` 等）の実行をスキップ。
   - `GetInteractPreCommand()`, `GetInteractPostCommand()`, `GetExitCommand()` を空文字 `""` に固定し、端末画面へのゴミ出力や不要な切断コマンド送信を完全防止。

---

## 3. 実装内容

### 3.1 変更ファイル一覧

| ファイル | 役割と主な変更内容 |
| :--- | :--- |
| `src/node/node.go` | - `Node` インターフェースに `IsDirectMode() bool` を追加<br>- `BaseNode`, `SshNode`, `TelnetNode` に `DirectMode` フィールドおよび `WithDirectMode(enable bool)` を追加<br>- `DirectMode` 有効時に Pre/Post/Exit コマンドを空文字 `""` に無力化 |
| `src/session/session.go` | - `Output() string` メソッドを追加<br>- `DirectSshLogin(node nd.Node) error` および `DirectTelnetLogin(node nd.Node) error` の新設<br>- 多段階インターバル出力判定エンジン `directLoginLoop` の実装<br>- `Ssh()` / `Telnet()` で `node.IsDirectMode()` による自動ルーティング |
| `src/session/session_wrap.go` | - `RunDirectSsh` および `RunDirectTelnet` ラッパー関数の新設<br>- `RunSession` 終了時の `exit` コマンド送信に空文字ガードを追加 |
| `goplur.go` | - `RunDirectSsh`, `RunDirectTelnet` をトップレベルパッケージに re-export |
| `src/node/node_test.go` | - `TestNode_DirectMode` によるプロパティ・ビルダーの単体テスト |
| `src/session/session_direct_test.go` | - FortiGate、パスワード認証、遅延ホスト、ATEN メニュー画面、無音タイムアウトの網羅的テスト |
| `host-credential-manager-go/hcm-client/main.go` | - `connectSSH`, `connectTelnet` に `WithDirectMode(true)` および `WithoutCommands()` を適用 |

---

## 4. 使い方と実装例

### 4.1 基本的な使い方（`WithDirectMode` を付与する）

最もシンプルな方法は、ノード生成時に `.WithDirectMode(true)` を付与することです。

```go
package main

import (
	"fmt"
	"log"

	"goplur"
)

func main() {
	// FortiGate への SSH 接続例
	node := goplur.NewSshNode("fg-core", "192.168.1.1", "admin", "password123", "fortinet").
		WithDirectMode(true)

	logParams := goplur.DefaultLogParams()

	// RunSsh を呼ぶだけで、自動的に DirectSshLogin が使用される
	err := goplur.RunSsh(node, &logParams, func(s *goplur.Session) error {
		fmt.Println("Connected! Starting interactive terminal...")
		// WithoutCommands() を指定して余計な stty 送信を防止
		return s.Interact(goplur.WithoutCommands())
	})

	if err != nil {
		log.Fatalf("Session failed: %v", err)
	}
}
```

### 4.2 ワンライナー関数 `RunDirectSsh` / `RunDirectTelnet` の利用

ノード側でフラグを設定しなくても、ラッパー関数を使うことで自動的に DirectMode を有効化して接続できます。

```go
node := goplur.NewSshNode("aten-kvm", "10.0.0.50", "admin", "secret", "aten")

err := goplur.RunDirectSsh(node, nil, func(s *goplur.Session) error {
	return s.Interact(goplur.WithoutCommands())
})
```

### 4.3 Telnet 接続におけるエスケープ切断との組み合わせ

ATEN やネットワーク機器で、ログアウトに `Ctrl+]` → `quit` が必要な場合でも、`WithEscapeExit()` と完全に共存できます。

```go
node := goplur.NewTelnetNode("core-sw", "192.168.10.1", "admin", "cisco", "cisco").
	WithDirectMode(true).
	WithEscapeExit()

err := goplur.RunTelnet(node, nil, func(s *goplur.Session) error {
	return s.Interact(goplur.WithoutCommands())
})
```

---

## 5. 検証結果

`src/session/session_direct_test.go` において、擬似リモートスクリプトを用いた全パターンの検証を実施済みです。

| テストケース | 検証内容 | 結果 | 所要時間 |
| :--- | :--- | :---: | :---: |
| `TestDirectLogin_FastPath_FortiGate` | FortiGate の `hostname (global) # ` プロンプトを Fast-Path で即時検知 | **PASS** | 0.00s |
| `TestDirectLogin_PasswordAuth` | `Password:` プロンプト検知後、パスワード送信を経て即座に対話開始 | **PASS** | 0.00s |
| `TestDirectLogin_DelayedHost` | 初期 2.5 秒の無音遅延（DNS逆引き）で誤判定せず、リトライによりパスワード要求を安全に検知 | **PASS** | 2.50s |
| `TestDirectLogin_AtenMenu` | 記号のない ATEN メニュー画面を 2 秒インターバル判定で確実に検知し対話開始 | **PASS** | 2.00s |
| `TestDirectLogin_NoResponseTimeout` | 完全無音ホストに対し、リトライ上限（2秒×4回=計8秒）到達時に適切にエラー終了 | **PASS** | 8.00s |

---

## 6. まとめ

DirectMode の導入により、以下のメリットが確立されました：

1. **完全なプロンプト非依存**: FortiGate, ATEN, Cisco, Linux, Windows, その他あらゆるアプライアンス機器に対して、個別の正規表現を定義・保守する必要が一切なくなりました。
2. **パスワード認証と鍵認証の完全透過**: パスワードプロンプトが出るホストも、鍵認証で即座に画面が出るホストも、同一のコードで自動処理されます。
3. **遅延・DNS逆引き耐性**: 接続初期の無音期間を多段階インターバル判定で安全に耐え抜き、誤検知による画面崩れを防止します。
4. **クリーンな端末制御**: 余計な `stty` コマンドや `exit` コマンドの誤送信を排除し、アプライアンス機器のセッションを汚染しません。
5. **完全な下位互換性**: 従来の `RunSsh` や `Session.Run()` による構成自動化機能には一切影響を与えず、対話用途に特化したモードとして安全に利用できます。
