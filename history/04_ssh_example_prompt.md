# SSHサンプルプログラムの対話的入力機能の計画

`example/ssh.go` において、事前に公開鍵が転送されている（`ssh-copy-id` 済み）前提で、ホスト名とアクセスIPを対話的に入力して簡単に実行できるようにする計画です。

---

## 1. 目的と変更内容
- Linux環境での実行を想定し、プログラム起動時に入力の手順（`ssh-copy-id` の利用推奨）を自然な英語で表示します。
- `hostname` と `accessIp` について、標準入力（Stdin）からユーザーが入力を与えられるようにプロンプトを追加します。何も入力せずに Enter を押した場合はデフォルト値（`localhost`、`127.0.0.1`）を使用します。
- その他のパラメータ（`username`、`password`、`port`、`platform`）はデフォルト値を踏襲します。
- この使い方を説明する10行以内の英語のコメントを `example/ssh.go` の先頭に追記します。

---

## 2. ユーザーレビューが必要な点（User Review Required）
> [!NOTE]
> ユーザーへの提示メッセージの文言（英語）として以下を使用します。
> ```
> Before running this example, please configure passwordless SSH access to the
> target host using 'ssh-copy-id'. Once configured, simply provide the target
> hostname and access IP address when prompted.
> ```

---

## 3. オープンな質問（Open Questions）
特にありません。

---

## 4. 予定されるコード変更

### goplur
#### [MODIFY] example/ssh.go
- `flag` パッケージの利用を廃止し、`bufio.Reader` を用いて標準入力から `hostname` と `accessIp` を読み込むように変更します。
- ファイルの先頭に使い方を説明するコメントを追加します。

```go
// This example demonstrates how to establish an SSH session using a local system user
// and passwordless SSH key authentication.
//
// Prerequisites:
// 1. You must copy your SSH public key to the target host beforehand using:
//    ssh-copy-id <user>@<access_ip>
// 2. Run this program, and enter the target hostname and access IP when prompted.
```

具体的な差分イメージ：
```go
// ... 冒頭のコメント追加 ...

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("----------------------------------------------------------------------")
	fmt.Println("Before running this example, please configure passwordless SSH access to the")
	fmt.Println("target host using 'ssh-copy-id'. Once configured, simply provide the target")
	fmt.Println("hostname and access IP address when prompted.")
	fmt.Println("----------------------------------------------------------------------")

	fmt.Print("Enter target Hostname (e.g., myhost) [default: localhost]: ")
	hostInput, _ := reader.ReadString('\n')
	hostname := strings.TrimSpace(hostInput)
	if hostname == "" {
		hostname = "localhost"
	}

	fmt.Print("Enter target Access IP (e.g., 192.168.1.100) [default: 127.0.0.1]: ")
	ipInput, _ := reader.ReadString('\n')
	accessIp := strings.TrimSpace(ipInput)
	if accessIp == "" {
		accessIp = "127.0.0.1"
	}

	username := os.Getenv("USER")
	port := 22
	platform := "ubuntu"
	password := ""

	// ... Nodeの初期化とRunSsh呼び出し ...
}
```

---

## 5. 検証計画

### 自動テスト
コード変更後もビルドや他のテストに影響がないことを確認するため、テストスイートを実行します。
```bash
go test -v ./...
```

### 手動確認
`go run example/ssh.go` を実行し、メッセージが表示されること、および Enter キーを押すことでデフォルト値のまま処理が試行されることを確認します。

---

## 6. 完了のまとめ（Walkthrough）

### 実施した変更内容

#### `example/ssh.go` の対話的入力化
- コマンドライン引数（flags）の利用を廃止し、起動時に `ssh-copy-id` の利用推奨メッセージを表示した上で、`hostname` および `accessIp` を標準入力から対話的に入力できるように変更しました。
- ファイルの先頭に、このサンプルの事前準備（公開鍵転送）と対話的実行に関する説明コメント（10行以内）を追加しました。

### 検証結果

#### 1. 自動テスト
`go test -v ./...` を実行し、すべてのテストが問題なくパスすることを確認しました。

#### 2. サンプルビルド確認
`go build example/ssh.go` がコンパイルエラーなく正常にビルドできることを検証しました。
