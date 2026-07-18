[English Version](./README.md)

# goplur

> [!NOTE]
> **謝辞と感謝:**
> `goplur` における対話型セッション制御および期待値マッチング（Expect）のコアエンジンは、[google/goexpect](https://github.com/google/goexpect) ライブラリからコードを抽出・簡素化したものをベースに構築されています。素晴らしいライブラリを開発・公開してくださった `goexpect` の原著者に深く感謝いたします。抽出されたサブセットは本プロジェクト内（`goplur/goexpect.go`）にインライン化されており、デフォルトの2秒のポーリング遅延を解消したミリ秒単位の超高速な動作と、外部依存ライブラリ（gRPCやGoのSSHクライアント等）の完全な排除を実現しています。

`goplur` は、様々なプラットフォームにわたるインタラクティブなセッションの管理や、冪等性のあるシェル操作を実行するための、Go製 CLI 自動化ツールです。元々は `github.com/google/goexpect` をベースに構築されており、Python の `plur` ライブラリの Go 移植版にあたります。

SSH、Telnet、ローカルの Bash セッションを自動化し、期待値のマッチング、動的な特権昇格、安全なログ書き込みなどを実行するための高レベルな API を提供します。

## 機能

- **セッション管理**: スタックベースのノード設計により、ネストされたセッション（SSH、Telnet、SU、SUDO）を容易に管理。
- **対話型オートメーション**: 設定可能な期待値シーケンスを使用して、SSH鍵の確認やパスワード入力などの複雑な対話型プロンプトを自動処理。
- **セキュアログ**: パスワードプロンプト中は自動的にログ（標準出力およびファイル出力）を一時ミュートし、機密情報の露出を回避。
- **冪等操作**: 一般的なシステム管理タスク（`sed` によるファイル編集、`yum`/`dnf` によるパッケージ管理、バックアップ作成、および `HereDoc` 設定）向けの冪等性のあるヘルパーメソッドを内蔵。
- **クロスプラットフォーム対応**: 様々な Linux ディストリビューション（AlmaLinux、CentOS、Ubuntu、Arch Linux）を自動検出して対応。

---

## インストール

Go プロジェクトに `goplur` を追加します。

```bash
go get goplur
```

*(リモートパッケージとしてインポートする場合は `go get github.com/pluripotency/goplur` を使用)*

---

## 基本的な使い方

### 1. ローカルの Bash セッション
ローカルの Bash セッションを開始し、コマンドを実行してファイルの有無を確認する例：

```go
package main

import (
	"log"
	"goplur"
)

func main() {
	// ローカルの "Me" ノードを初期化
	node := goplur.NewMeNode()

	// ローカルの bash セッションを開始して操作をラップ
	err := goplur.RunBash(node, nil, func(s *goplur.Session) error {
		// シンプルなコマンドの実行
		output, err := s.Run("ls -la")
		if err != nil {
			return err
		}
		log.Printf("Current Directory:\n%s", output)

		// ファイルのバックアップを作成
		_, err = s.CreateBackup("/tmp/important_file.txt", ".org", false)
		return err
	})
	if err != nil {
		log.Fatalf("Session failed: %v", err)
	}
}
```

### 2. ネストされた SSH セッションと特権昇格
カスタムのターゲットノードをセッションスタックに追加し、SSH/Telnet でログインして、`Sudo` や `Su` で動的に `root` 特権に昇格する例：

```go
package main

import (
	"log"
	"goplur"
)

func main() {
	// ターゲットとなる SSH ホストノードの作成
	sshNode := goplur.NewSshNode("webserver", "192.168.10.22", "admin", "mySecretPassword", "almalinux9")

	// SSH セッションを開始
	err := goplur.RunSsh(sshNode, nil, func(s *goplur.Session) error {
		
		// 'admin' ユーザーとしてコマンドを実行
		s.Run("whoami") // "admin" が返る

		// Sudo ラッパーを使用して root 特権に昇格
		err := goplur.Sudo(s, func(s *goplur.Session) error {
			s.Run("whoami") // "root" が返る
			
			// 冪等な行追加
			return s.AppendLine("export APP_ENV=production", "/etc/profile")
		})
		
		return err
	})
	if err != nil {
		log.Fatalf("SSH execution failed: %v", err)
	}
}
```

---

## 冪等（べきとう）操作ヘルパー

`goplur` では、`Session` 構造体に直接バインドされた多数の冪等な操作ヘルパーを提供しています。

| メソッド | 説明 |
| :--- | :--- |
| `Run(cmd)` | コマンドを実行し、キャプチャされた出力を返します。 |
| `Wget(url, opts)` | `wget` を実行し、ダウンロードの成否を返します（404エラーやルーティングエラーも安全に処理）。 |
| `Patch(patchfile)` | `patch` を安全に実行します（逆パッチや適用済みのパッチを検知した場合は自動的に拒否）。 |
| `HereDoc(filePath, contents, EOF)` | 指定したパスのファイルに複数行の HereDoc を生成・書き込みします。 |
| `SedReplace(srcExp, dstStr, srcFile, dstFile)` | `sed` を使用してファイル内の正規表現にマッチする文字列を置換します。 |
| `AppendLine(line, filePath)` | 指定した行がファイル内に存在しない場合のみ、その行を末尾に追加します。 |
| `CheckFileExists(filePath)` | ホスト上にファイルが存在するかどうかを確認します。 |
| `CheckCommandExists(cmd)` | CLI コマンドがインストールされ、`$PATH` から利用可能であるか確認します。 |
| `ServiceOn(serviceName)` | systemd サービスを即座に有効化して起動します（`systemctl enable --now`）。 |

---

## ログ設定

ログの動作は、環境変数 `LOG_PARAMS` または初期化時に渡されるプロパティによって制御されます。

```bash
# 標準出力のみにログを出力（デフォルト）
export LOG_PARAMS=only_stdout

# /tmp/plur_log に出力ログとデバッグトレースファイルを書き出す
export LOG_PARAMS=normal

# ログを書き出し、マッチしなかったバッファサイズもデバッグトレースに残す
export LOG_PARAMS=debug
```

利用可能な `LogParams` 構造体のフィールド:
- `LogDir`: 出力ログとデバッグトレースを保存するディレクトリ。
- `EnableStdout`: 実行中のプロセス出力を live で `os.Stdout` に書き出す。
- `OutputLogFilePath` / `OutputLogAppendPath`: 出力ログの追跡パス。
- `DebugLogFilePath` / `DebugLogAppendPath`: 期待値マッチング、アクション、および実行時間の追跡トレースパス。
- `DebugColor`: カラフルなログ出力の有効化。
- `DeleteMtime` / `DeleteMtimeUnit`: セッション開始時に `LogDir` 内の古いファイルを自動削除する（例: 10日以上前のファイルを削除）。
