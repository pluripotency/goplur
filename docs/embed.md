# ワンバイナリ埋め込みホストリストとインクリメンタルサーチ接続の設計仕様書

本書では、SSH 接続先や Telnet 接続先を定義したホストリスト（および必要な秘密鍵群）を `//go:embed` でバイナリ内部に完全に固め、実行時にインクリメンタルサーチ（リアルタイム絞り込み検索）で接続先を選択して、ユーザー名・パスワード・秘密鍵を手動入力することなく一発で安全に対話接続できる設計仕様を定義します。

---

## 1. 背景と課題

### 1.1 運用現場における課題
サーバー群（Web, DB, バッチ等）やネットワーク機器（スイッチ, ルータ）への運用アクセスにおいて、以下のような課題が日常的に発生しています：
- **接続情報の散乱**: ホスト名、アクセスIP、ポート番号、踏み台情報、ログインユーザー名がスプレッドシートや個人メモに散らばっている。
- **秘密鍵・認証情報の管理負担**: 接続先ごとに異なる秘密鍵（`.pem`, `id_rsa`）の配置場所の管理や、`chmod 600` の設定ミスによる SSH 接続エラーの頻発。
- **多段・特殊接続の手間**: Telnet の Cisco 特権モード（`enable`）や、`Ctrl+]` でしか切断できない特殊アプライアンスへの接続手順が属人化している。
- **配布とポータビリティ**: 踏み台サーバーや運用端末が変わるたびに、スクリプトと鍵ファイル群を同期・配置し直す必要がある。

### 1.2 目指すソリューション
Go 1.16+ で導入された **`//go:embed`** を活用し、ホスト一覧定義（JSON/YAML）および必要な秘密鍵ファイルを**単一の実行可能バイナリ（ワンバイナリ）**に内包します。
配布されたバイナリを実行すると、端末上にインタラクティブなインクリメンタルサーチ画面が現れ、ホスト名・IP・タグ等で即座に絞り込んで Enter を押すだけで、認証情報の入力なしに自動ログインし、即座に対話シェル（`s.Interact()`）へ移行します。

---

## 2. 全体アーキテクチャ

```
+-------------------------------------------------------------------------+
|                  ワンバイナリ (Single Executable Binary)                 |
|                                                                         |
|  [ 埋め込みリソース (go:embed) ]                                         |
|    - hosts.json  (ホスト一覧・認証プロファイル・タグ・接続方式)             |
|    - keys/*      (対象ホスト用の秘密鍵ファイル群: id_rsa_prod, id_rsa_dev)  |
|                                                                         |
|                                    v                                    |
|  [ 1. 設定ローダー & バリデータ ]                                        |
|    - JSON のパースと構造体マッピング                                     |
|    - 埋め込み秘密鍵の存在検証                                            |
|                                                                         |
|                                    v                                    |
|  [ 2. インクリメンタルサーチ エンジン (TUI) ]                            |
|    - ターミナルの Raw モード化                                           |
|    - リアルタイム入力フィルタ (ホスト名 / IP / タグの部分一致 & AND検索)    |
|    - 上下カーソル移動とハイライト選択                                    |
|                                                                         |
|                                    v (Enter で選択)                     |
|  [ 3. ノード動的構築 & 秘密鍵テンポラリ展開 ]                            |
|    - SSH 鍵認証の場合: メモリ上の鍵データをテンポラリファイル (0600) へ展開 |
|      (セッション終了時に defer で確実に自動削除)                          |
|    - goplur.SshNode / goplur.TelnetNode のインスタンス生成               |
|    - WithEscapeExit() や ConnectHandler の適用                           |
|                                                                         |
|                                    v                                    |
|  [ 4. goplur セッション実行 & 対話モード ]                              |
|    - goplur.RunSsh / goplur.RunTelnet による自動ログイン                 |
|    - ログイン完了後、即座に s.Interact() へ移行しユーザーに対話権を委譲  |
|    - exit または Ctrl+] で切断後、リソースを安全にクリーンアップして終了 |
+-------------------------------------------------------------------------+
```

---

## 3. データモデル設計（Host Configuration Schema）

ホスト一覧を定義する `hosts.json` のスキーマ設計です。SSH と Telnet の双対性および、Cisco 特権昇格や Telnet エスケープ切断等のカスタム設定を吸収できるように設計します。

```json
[
  {
    "id": "web-prod-01",
    "name": "Production Web Frontend #1",
    "type": "ssh",
    "access_ip": "10.0.1.11",
    "port": 22,
    "username": "deploy",
    "auth_type": "key",
    "key_file": "keys/id_rsa_prod",
    "use_login_flag": true,
    "platform": "almalinux9",
    "tags": ["prod", "web", "tokyo"]
  },
  {
    "id": "db-staging",
    "name": "Staging PostgreSQL Primary",
    "type": "ssh",
    "access_ip": "10.0.2.15",
    "port": 2222,
    "username": "postgres",
    "auth_type": "password",
    "password": "stagingPassword123",
    "platform": "ubuntu",
    "tags": ["stg", "db"]
  },
  {
    "id": "core-router-01",
    "name": "Datacenter Core Router #1",
    "type": "telnet",
    "access_ip": "192.168.100.1",
    "port": 23,
    "username": "admin",
    "password": "telnetPassword",
    "enable_password": "ciscoEnableSecret",
    "platform": "cisco",
    "escape_exit": true,
    "tags": ["network", "router", "cisco"]
  }
]
```

### フィールド定義

| フィールド | 型 | 必須 | 説明 |
| :--- | :--- | :---: | :--- |
| `id` | string | ○ | 一意のホスト識別子 |
| `name` | string | ○ | 画面表示用の分かりやすい名称 |
| `type` | string | ○ | `"ssh"` または `"telnet"` |
| `access_ip` | string | ○ | 接続先 IP アドレスまたはホスト名 |
| `port` | int | - | 接続ポート（SSH: デフォルト 22 / Telnet: デフォルト 23） |
| `username` | string | ○ | ログインユーザー名 |
| `auth_type` | string | - | SSH の認証種別（`"key"` または `"password"`） |
| `key_file` | string | - | `auth_type: "key"` の場合の埋め込み秘密鍵パス（バイナリ内相対パス） |
| `password` | string | - | パスワード（SSH パスワード認証、または Telnet ログイン用） |
| `enable_password` | string | - | Cisco 機器等の特権モード（`enable`）昇格用パスワード |
| `use_login_flag` | bool | - | `ssh -l user` 形式を使用するかどうか |
| `escape_exit` | bool | - | Telnet 切断時に `Ctrl+]` → `quit` シーケンスを使用するかどうか |
| `platform` | string | - | プラットフォーム種別（`"ubuntu"`, `"almalinux9"`, `"cisco"`, `"generic"` 等） |
| `tags` | []string | - | 絞り込み検索用タグの配列 |

---

## 4. コアコンポーネント設計

### 4.1 リソース埋め込み（`//go:embed`）
バイナリのルートパッケージまたは専用パッケージで、設定ファイルと秘密鍵ディレクトリを埋め込みます。

```go
package main

import (
	"embed"
)

//go:embed hosts.json
var embeddedHostsJSON []byte

//go:embed keys/*
var embeddedKeysFS embed.FS
```

- **メリット**:
  - 設定ファイルと秘密鍵ファイル群が `.rodata` セクションに直接コンパイルされます。
  - 配布時に `hosts.json` や鍵ファイルを個別に同梱・転送する必要が一切なくなります。

---

### 4.2 秘密鍵の安全な一時展開ライフサイクル
SSH クライアントコマンド（`/usr/bin/ssh`）はディスク上のファイルパス（`-i <path>`）を要求し、かつパーミッションが `0600`（所有者のみ読み書き可）であることを厳格に検証します。

埋め込まれた秘密鍵データを安全に SSH コマンドへ渡すため、**テンポラリ展開パターン** を採用します：

```
[メモリ上の秘密鍵データ (embed.FS)]
              │
              ▼ os.CreateTemp("", "goplur-key-*")
[テンポラリファイル作成 (例: /tmp/goplur-key-123456)]
              │
              ▼ os.Chmod(tempFile.Name(), 0600)  <-- SSH が要求する厳格な権限
[アクセス権限 0600 設定]
              │
              ▼ sshNode.WithKey(tempPath) でセッション開始
[SSH コマンド実行]
              │
              ▼ defer os.Remove(tempPath)
[セッション終了後、ディスクから完全に即座消去]
```

#### 実装上の安全性ルール
1. **最小生存期間**: 接続直前に作成し、`defer os.Remove(tmpPath)` によってセッション終了時に例外なく削除する。
2. **パーミッションの事前制限**: 作成直後に即座に `0600` を設定し、他ユーザーからの読み取りを遮断する。
3. **シグナル中断ハンドリング**: ユーザーが `Ctrl+C` 等でプロセスを強制中断した場合でも、シグナルトラップ（`os/signal`）経由で一時ファイルを確実にクリーンアップする。

---

### 4.3 インクリメンタルサーチ（リアルタイム絞り込み検索 UI）
外部の `fzf` や `peco` などの CLI ツールに依存せず、純粋な Go 標準ライブラリ（`golang.org/x/term` 等）を用いてバイナリ単体で動作するインクリメンタルサーチエンジンを設計します。

#### UI 仕様
- **プロンプト**: `Search> <ユーザー入力>`
- **検索アルゴリズム**:
  - スペース区切りによる **AND 検索**（例: `web prod tokyo`）。
  - 各トークンは `name`, `access_ip`, `username`, `tags` に対して大文字小文字を区別しない部分一致（Case-insensitive Partial Match）。
- **キーバインド**:
  - `文字キー`: リアルタイム絞り込み（入力のたびに候補リストが即時再描画される）
  - `Backspace`: 1文字削除
  - `↑` / `Ctrl+P`: 選択候補を上に移動
  - `↓` / `Ctrl+N`: 選択候補を下に移動
  - `Enter`: 現在ハイライトされているホストを決定し接続開始
  - `ESC` / `Ctrl+C`: 検索を中断し終了

---

### 4.4 goplur セッションへの自動バインディング

インクリメンタルサーチで選択されたホスト情報から、`goplur` の各ノード型を生成し、適切なオプションを設定して実行します。

#### (1) SSH ホストのバインディング
```go
func connectSSH(host HostConfig) error {
	node := goplur.NewSshNode(host.ID, host.AccessIP, host.Username, host.Password, host.Platform)
	if host.Port != 0 {
		node.SSHPort = host.Port
	}
	if host.UseLoginFlag {
		node.WithLoginFlag(true)
	}

	// 秘密鍵認証の場合の一時展開
	if host.AuthType == "key" && host.KeyFile != "" {
		keyData, err := embeddedKeysFS.ReadFile(host.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to read embedded key %s: %w", host.KeyFile, err)
		}

		tmpKeyFile, err := os.CreateTemp("", "goplur-key-*")
		if err != nil {
			return err
		}
		defer os.Remove(tmpKeyFile.Name()) // 終了時に確実に削除

		if err := os.Chmod(tmpKeyFile.Name(), 0600); err != nil {
			return err
		}
		if _, err := tmpKeyFile.Write(keyData); err != nil {
			return err
		}
		tmpKeyFile.Close()

		node.WithKey(tmpKeyFile.Name())
	}

	// ログ設定とセッション実行
	logParams := goplur.DefaultLogParams()
	return goplur.RunSsh(node, &logParams, func(s *goplur.Session) error {
		fmt.Printf("\n[+] Successfully connected to %s (%s)!\n", host.Name, host.AccessIP)
		return s.Interact() // 即座に対話モードへ移行
	})
}
```

#### (2) Telnet ホストのバインディング（Cisco 特権昇格・エスケープ切断対応）
```go
func connectTelnet(host HostConfig) error {
	node := goplur.NewTelnetNode(host.ID, host.AccessIP, host.Username, host.Password, host.Platform)
	if host.Port != 0 {
		node.TelnetPort = host.Port
	}

	// Telnet エスケープ切断シーケンス (Ctrl+] -> quit)
	if host.EscapeExit {
		node.WithEscapeExit()
	}

	// Cisco 特権モード (enable) が定義されている場合
	if host.EnablePassword != "" {
		node.WaitPrompt = fmt.Sprintf(`%s#`, host.ID)
		node.ConnectHandler = func(s goplur.SessionExecutor, n goplur.Node) error {
			sess := s.(*goplur.Session)
			if err := sess.DefaultTelnetLogin(n); err != nil {
				return err
			}
			rows := []goplur.ExpectRow{
				{Pattern: `[Pp]assword:`, Reaction: goplur.ReactionSendPass, Arg: host.EnablePassword, Label: "enable pass"},
				{Pattern: node.GetWaitPrompt(), Reaction: goplur.ReactionSuccess, Arg: true, Label: "privileged prompt"},
			}
			_, err := sess.Do("enable", rows, 5*goplur.DefaultTimeout)
			return err
		}
		node.ExitHandler = func(s goplur.SessionExecutor, _ goplur.Node) error {
			sess := s.(*goplur.Session)
			_, _ = sess.Run("disable")
			return sess.SendLine("exit")
		}
	}

	logParams := goplur.DefaultLogParams()
	return goplur.RunTelnet(node, &logParams, func(s *goplur.Session) error {
		fmt.Printf("\n[+] Successfully connected to %s (%s)!\n", host.Name, host.AccessIP)
		return s.Interact()
	})
}
```

---

## 5. セキュリティ考慮事項

1. **バイナリに含まれる機密情報の保護**:
   - `//go:embed` されたデータ（秘密鍵やパスワード文字列）は、コンパイル後のバイナリ内にプレーンテキストとして存在します。
   - `strings` コマンド等でバイナリを走査すると情報が抽出できるため、以下の対策が推奨されます：
     - **バイナリの実行権限制限**: `chmod 700 <binary>` 等で特定管理者のみに実行権限を絞る。
     - **埋め込み暗号化（オプション）**: 機密データ部を暗号化しておき、バイナリ起動時に環境変数（`MASTER_KEY`）やパスフレーズ入力によって復号してメモリ展開する構造。
2. **一時秘密鍵の漏洩防止**:
   - 一時ファイルは常に `0600` で作成し、他のOSユーザーからの読み取りをブロックする。
   - プロセス終了フック（`defer`）および `SIGINT` / `SIGTERM` シグナルハンドラーにより、異常終了時でも一時ファイルが `/tmp` に残留しないようにする。

---

## 6. まとめ

- **ポータビリティの極大化**: 設定ファイル・秘密鍵・接続ロジック・検索 TUI がすべて 1 つのバイナリに凝縮され、SCP 等で配置するだけで即座に利用可能。
- **入力ストレスの完全排除**: 接続先をインクリメンタルサーチで選択するだけで、ユーザー名・パスワード・秘密鍵の指定を全自動化。
- **goplur の強みを全面活用**: SSH だけでなく、Cisco 特権昇格や Telnet の `Ctrl+]` 切断など、複雑なネットワーク機器の接続ライフサイクルまで同一の UI で透過的に操作可能。
