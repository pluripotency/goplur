# ワンバイナリ埋め込みホストリスト & インクリメンタルサーチ接続 実装例

本書では、[`docs/embed.md`](file:///home/worker/Documents/antigravity/goplur/docs/embed.md) で定義した設計仕様に基づき、**ホスト定義リスト（JSON）と秘密鍵群を `//go:embed` でワンバイナリに固め、インクリメンタルサーチで選択して自動接続する完全な実装例** を解説します。

---

## 1. ディレクトリ構成

ワンバイナリツールを構築するための推奨構成です：

```
example/embed/
├── main.go            # メインプログラム（embed, TUIインクリメンタルサーチ, goplur連携）
├── hosts.json         # 埋め込む接続先ホスト一覧
└── keys/              # 埋め込む秘密鍵ディレクトリ
    ├── id_rsa_web     # Webサーバー用秘密鍵
    └── id_rsa_prod    # 本番環境用秘密鍵
```

---

## 2. 実装ファイル

### 2.1 ホスト一覧定義 (`example/embed/hosts.json`)

SSH（鍵認証・パスワード認証）、および Telnet（Cisco 特権昇格・エスケープ切断対応）のホストを定義します。

```json
[
  {
    "id": "web-tokyo-01",
    "name": "Production Web Frontend 01 (Tokyo)",
    "type": "ssh",
    "access_ip": "10.0.1.11",
    "port": 22,
    "username": "deploy",
    "auth_type": "key",
    "key_file": "keys/id_rsa_web",
    "use_login_flag": true,
    "platform": "ubuntu",
    "tags": ["prod", "web", "tokyo", "aws"]
  },
  {
    "id": "db-master-01",
    "name": "Primary Database Master (Osaka)",
    "type": "ssh",
    "access_ip": "10.0.2.50",
    "port": 2222,
    "username": "postgres",
    "auth_type": "password",
    "password": "mySecureDbPassword",
    "platform": "almalinux9",
    "tags": ["prod", "db", "osaka", "postgresql"]
  },
  {
    "id": "core-sw-01",
    "name": "Datacenter Core Switch (L3)",
    "type": "telnet",
    "access_ip": "192.168.10.1",
    "port": 23,
    "username": "admin",
    "password": "switchPassword",
    "enable_password": "ciscoEnableSecret",
    "platform": "cisco",
    "escape_exit": true,
    "tags": ["network", "switch", "cisco", "datacenter"]
  },
  {
    "id": "local-sandbox",
    "name": "Local Test Machine",
    "type": "ssh",
    "access_ip": "127.0.0.1",
    "port": 22,
    "username": "worker",
    "auth_type": "password",
    "password": "workerPassword",
    "platform": "ubuntu",
    "tags": ["local", "test", "dev"]
  }
]
```

---

### 2.2 メインソースコード (`example/embed/main.go`)

外部の重い TUI ライブラリを使わず、Go 標準ライブラリと `golang.org/x/term` だけで動作する、軽量かつ高速なインクリメンタルサーチと `goplur` 接続ロジックの完全なコードです。

```go
package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"

	"goplur"
)

// 1. リソースの埋め込み (ワンバイナリ化のコア)
//
//go:embed hosts.json
var embeddedHostsData []byte

//go:embed keys/*
var embeddedKeysFS embed.FS

// HostConfig は hosts.json のスキーマ定義です
type HostConfig struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"` // "ssh" or "telnet"
	AccessIP       string   `json:"access_ip"`
	Port           int      `json:"port"`
	Username       string   `json:"username"`
	AuthType       string   `json:"auth_type"` // "key" or "password"
	KeyFile        string   `json:"key_file"`  // embeddedKeysFS 内のパス
	Password       string   `json:"password"`
	EnablePassword string   `json:"enable_password"` // Cisco 特権モード用
	UseLoginFlag   bool     `json:"use_login_flag"`
	EscapeExit     bool     `json:"escape_exit"` // Telnet Ctrl+] 切断
	Platform       string   `json:"platform"`
	Tags           []string `json:"tags"`
}

func main() {
	// ホスト定義のパース
	var hosts []HostConfig
	if err := json.Unmarshal(embeddedHostsData, &hosts); err != nil {
		log.Fatalf("Failed to parse embedded hosts.json: %v", err)
	}

	if len(hosts) == 0 {
		log.Fatal("No hosts configured in embedded hosts.json")
	}

	// 2. インクリメンタルサーチ UI の起動
	selectedHost, err := selectHostInteractive(hosts)
	if err != nil {
		fmt.Printf("\nSearch cancelled: %v\n", err)
		return
	}

	fmt.Printf("\n[+] Target Selected: %s (%s)\n", selectedHost.Name, selectedHost.AccessIP)
	fmt.Printf("[+] Protocol: %s, User: %s, Port: %d\n", selectedHost.Type, selectedHost.Username, resolvePort(selectedHost))
	fmt.Println("[+] Connecting automatically without asking for credentials...")

	// 3. 選択されたホストへ自動接続
	switch selectedHost.Type {
	case "ssh":
		err = connectSSH(selectedHost)
	case "telnet":
		err = connectTelnet(selectedHost)
	default:
		log.Fatalf("Unsupported host type: %s", selectedHost.Type)
	}

	if err != nil {
		log.Fatalf("\nConnection failed: %v", err)
	}
	fmt.Println("\n[+] Session disconnected successfully.")
}

func resolvePort(h HostConfig) int {
	if h.Port != 0 {
		return h.Port
	}
	if h.Type == "telnet" {
		return 23
	}
	return 22
}

// --------------------------------------------------------------------------------
// 接続ハンドラー
// --------------------------------------------------------------------------------

func connectSSH(host HostConfig) error {
	node := goplur.NewSshNode(host.ID, host.AccessIP, host.Username, host.Password, host.Platform)
	if host.Port != 0 {
		node.SSHPort = host.Port
	}
	if host.UseLoginFlag {
		node.WithLoginFlag(true)
	}

	// 秘密鍵認証の場合: メモリ上の埋め込みデータを一時ファイル (0600) に安全に展開
	if host.AuthType == "key" && host.KeyFile != "" {
		keyBytes, err := embeddedKeysFS.ReadFile(host.KeyFile)
		if err != nil {
			return fmt.Errorf("embedded key %q not found: %w", host.KeyFile, err)
		}

		tmpFile, err := os.CreateTemp("", "goplur-key-*")
		if err != nil {
			return fmt.Errorf("failed to create temp key file: %w", err)
		}
		tmpPath := tmpFile.Name()

		// 終了時に確実に削除
		defer os.Remove(tmpPath)

		// 中断シグナル時にも即座にクリーンアップ
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			_ = os.Remove(tmpPath)
			os.Exit(1)
		}()

		// 権限 0600 を設定して書き込み
		if err := os.Chmod(tmpPath, 0600); err != nil {
			return err
		}
		if _, err := tmpFile.Write(keyBytes); err != nil {
			return err
		}
		_ = tmpFile.Close()

		// ノードに展開した鍵パスを設定
		node.WithKey(tmpPath)
	}

	logParams := goplur.DefaultLogParams()

	// SSH セッション開始 -> 即座に Interact() へ移行
	return goplur.RunSsh(node, &logParams, func(s *goplur.Session) error {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf(" Connected to %s via SSH. Interactive shell ready.\n", host.Name)
		fmt.Println(" Type 'exit' or press Ctrl+D to disconnect.")
		fmt.Println("--------------------------------------------------------------------------------")
		return s.Interact()
	})
}

func connectTelnet(host HostConfig) error {
	node := goplur.NewTelnetNode(host.ID, host.AccessIP, host.Username, host.Password, host.Platform)
	if host.Port != 0 {
		node.TelnetPort = host.Port
	}

	// Telnet エスケープ切断シーケンス (Ctrl+] -> quit)
	if host.EscapeExit {
		node.WithEscapeExit()
	}

	// Cisco 特権モード (enable) が設定されている場合
	if host.EnablePassword != "" {
		node.WaitPrompt = fmt.Sprintf(`%s#`, host.ID)
		node.ConnectHandler = func(s goplur.SessionExecutor, n goplur.Node) error {
			sess := s.(*goplur.Session)
			// 1. 標準 Telnet ログイン
			if err := sess.DefaultTelnetLogin(n); err != nil {
				return err
			}
			// 2. enable 昇格シーケンス
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

	// Telnet セッション開始 -> 即座に Interact() へ移行
	return goplur.RunTelnet(node, &logParams, func(s *goplur.Session) error {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf(" Connected to %s via Telnet. Interactive terminal ready.\n", host.Name)
		if host.EscapeExit {
			fmt.Println(" Disconnect via exit or Telnet escape (Ctrl+], then quit).")
		}
		fmt.Println("--------------------------------------------------------------------------------")
		return s.Interact()
	})
}

// --------------------------------------------------------------------------------
// インクリメンタルサーチ (TUI) 実装
// --------------------------------------------------------------------------------

func selectHostInteractive(allHosts []HostConfig) (HostConfig, error) {
	// 標準入力を Raw モード化してキー入力を即時取得
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Raw モード非対応環境の場合はフォールバック簡易選択
		return fallbackSelect(allHosts)
	}
	defer term.Restore(fd, oldState)

	var query strings.Builder
	cursorIdx := 0

	render := func() []HostConfig {
		// 画面クリアと再描画
		fmt.Print("\r\x1b[2K\x1b[H") // カーソルをホームへ
		fmt.Print("\x1b[J")          // 画面クリア

		fmt.Print("=== QuickConnect Host Search (Type to filter, UP/DOWN to navigate, ENTER to select, ESC/Ctrl+C to quit) ===\r\n")
		fmt.Printf("Query> %s_\r\n\r\n", query.String())

		filtered := filterHosts(allHosts, query.String())
		if len(filtered) == 0 {
			fmt.Print("  [No matching hosts found]\r\n")
			return nil
		}

		if cursorIdx >= len(filtered) {
			cursorIdx = len(filtered) - 1
		}
		if cursorIdx < 0 {
			cursorIdx = 0
		}

		maxDisplay := 10
		start := 0
		if cursorIdx >= maxDisplay {
			start = cursorIdx - maxDisplay + 1
		}
		end := start + maxDisplay
		if end > len(filtered) {
			end = len(filtered)
		}

		for i := start; i < end; i++ {
			h := filtered[i]
			tagStr := strings.Join(h.Tags, ",")
			prefix := "  "
			if i == cursorIdx {
				// ハイライト表示 (反転表示)
				prefix = "\x1b[7m> "
			}

			line := fmt.Sprintf("%-16s | %-6s | %-15s | %-10s | %-25s | [%s]",
				h.ID, strings.ToUpper(h.Type), h.AccessIP, h.Username, h.Name, tagStr)

			if i == cursorIdx {
				fmt.Printf("%s%s\x1b[0m\r\n", prefix, line)
			} else {
				fmt.Printf("%s%s\r\n", prefix, line)
			}
		}

		fmt.Printf("\r\nShowing %d/%d hosts (Index: %d)\r\n", len(filtered), len(allHosts), cursorIdx+1)
		return filtered
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		filtered := render()

		b, err := reader.ReadByte()
		if err != nil {
			return HostConfig{}, err
		}

		switch b {
		case 3: // Ctrl+C
			return HostConfig{}, fmt.Errorf("interrupted")
		case 13: // Enter
			if len(filtered) > 0 && cursorIdx < len(filtered) {
				term.Restore(fd, oldState)
				return filtered[cursorIdx], nil
			}
		case 127, 8: // Backspace
			str := query.String()
			if len(str) > 0 {
				query.Reset()
				query.WriteString(str[:len(str)-1])
			}
		case 27: // Escape / Arrow keys sequence
			if reader.Buffered() >= 2 {
				b1, _ := reader.ReadByte()
				b2, _ := reader.ReadByte()
				if b1 == '[' {
					switch b2 {
					case 'A': // Up arrow
						cursorIdx--
						if cursorIdx < 0 {
							cursorIdx = 0
						}
					case 'B': // Down arrow
						cursorIdx++
						if len(filtered) > 0 && cursorIdx >= len(filtered) {
							cursorIdx = len(filtered) - 1
						}
					}
				}
			} else {
				// 単体 ESC は終了
				return HostConfig{}, fmt.Errorf("escape pressed")
			}
		case 16: // Ctrl+P (Up)
			cursorIdx--
			if cursorIdx < 0 {
				cursorIdx = 0
			}
		case 14: // Ctrl+N (Down)
			cursorIdx++
			if len(filtered) > 0 && cursorIdx >= len(filtered) {
				cursorIdx = len(filtered) - 1
			}
		default:
			// 通常の表示可能文字 (スペース含む)
			if b >= 32 && b <= 126 {
				query.WriteByte(b)
				cursorIdx = 0 // 絞り込みが変わったら先頭へ
			}
		}
	}
}

// filterHosts はスペース区切りによる AND 検索を実行します
func filterHosts(hosts []HostConfig, queryString string) []HostConfig {
	trimmed := strings.TrimSpace(queryString)
	if trimmed == "" {
		return hosts
	}

	tokens := strings.Fields(strings.ToLower(trimmed))
	var result []HostConfig

	for _, h := range hosts {
		targetText := strings.ToLower(fmt.Sprintf("%s %s %s %s %s %s",
			h.ID, h.Name, h.AccessIP, h.Username, h.Type, strings.Join(h.Tags, " ")))

		matchAll := true
		for _, token := range tokens {
			if !strings.Contains(targetText, token) {
				matchAll = false
				break
			}
		}
		if matchAll {
			result = append(result, h)
		}
	}

	return result
}

// fallbackSelect は非 TUI / パイプライン環境用のフォールバック選択です
func fallbackSelect(hosts []HostConfig) (HostConfig, error) {
	fmt.Println("Select a host by number:")
	for i, h := range hosts {
		fmt.Printf("[%d] %s (%s) [%s]\n", i+1, h.Name, h.AccessIP, h.Type)
	}
	fmt.Print("Enter number: ")
	var idx int
	if _, err := fmt.Scanln(&idx); err != nil || idx < 1 || idx > len(hosts) {
		return HostConfig{}, fmt.Errorf("invalid selection")
	}
	return hosts[idx-1], nil
}
```

---

## 3. ビルドとワンバイナリ生成手順

ワンバイナリ化は標準の `go build` コマンドを実行するだけで完了します。

```bash
# example/embed ディレクトリに移動
cd example/embed

# 単一バイナリ (quickconnect) をビルド
go build -o quickconnect main.go
```

生成された `quickconnect` のファイルサイズは約 10〜15MB 程度（Go ランタイム＋全鍵データ＋JSON を含む）となり、**外部ファイルに一切依存せず単体で配布・実行可能**です。

---

## 4. 実行画面と操作イメージ

### (1) 起動時のインクリメンタルサーチ画面
バイナリを実行すると、即座にターミナル全体が検索 TUI となり、ホスト一覧が表示されます。

```text
=== QuickConnect Host Search (Type to filter, UP/DOWN to navigate, ENTER to select, ESC/Ctrl+C to quit) ===
Query> _

> web-tokyo-01     | SSH    | 10.0.1.11       | deploy     | Production Web Frontend   | [prod,web,tokyo,aws]
  db-master-01     | SSH    | 10.0.2.50       | postgres   | Primary Database Master   | [prod,db,osaka,postgresql]
  core-sw-01       | TELNET | 192.168.10.1    | admin      | Datacenter Core Switch    | [network,switch,cisco,datacenter]
  local-sandbox    | SSH    | 127.0.0.1       | worker     | Local Test Machine        | [local,test,dev]

Showing 4/4 hosts (Index: 1)
```

---

### (2) リアルタイム絞り込み（例: `cisco` と入力）
文字を 1 文字入力するごとに、候補がリアルタイムに絞り込まれます。

```text
=== QuickConnect Host Search (Type to filter, UP/DOWN to navigate, ENTER to select, ESC/Ctrl+C to quit) ===
Query> cisco_

> core-sw-01       | TELNET | 192.168.10.1    | admin      | Datacenter Core Switch    | [network,switch,cisco,datacenter]

Showing 1/4 hosts (Index: 1)
```

スペース区切りで `web tokyo` のように複数キーワードを指定した **AND 検索** も可能です。

---

### (3) 選択と自動接続（Enter 押下時）
ホストを選択して `Enter` を押すと：
1. 秘密鍵がメモリから一時ファイル（パーミッション `0600`）に展開されます。
2. ユーザー名・パスワード・ポート・鍵オプションを自動付与して `goplur` セッションが起動します。
3. ログイン完了後、即座に対話モード（`s.Interact()`）へ移行し、直接シェル操作が可能になります。

```text
[+] Target Selected: Production Web Frontend 01 (Tokyo) (10.0.1.11)
[+] Protocol: ssh, User: deploy, Port: 22
[+] Connecting automatically without asking for credentials...
--------------------------------------------------------------------------------
 Connected to Production Web Frontend 01 (Tokyo) via SSH. Interactive shell ready.
 Type 'exit' or press Ctrl+D to disconnect.
--------------------------------------------------------------------------------
deploy@web-tokyo-01:~$ uname -a
Linux web-tokyo-01 5.15.0-101-generic #111-Ubuntu SMP x86_64 GNU/Linux
deploy@web-tokyo-01:~$ exit
logout

[+] Session disconnected successfully.
```

---

## 5. この実装のメリット

1. **ポータビリティ**: `hosts.json` や秘密鍵ファイルを個別に配置する手間がなくなり、バイナリ 1 つを踏み台サーバーに配置するだけで全メンバーが同じ環境で即座に接続できます。
2. **パスワード・鍵指定の完全自動化**: 複雑なパスワードや長い鍵ファイルパスを記憶・コピー＆ペーストする必要が一切ありません。
3. **セキュリティの担保**: 展開される一時秘密鍵ファイルは `0600` 権限で保護され、セッション終了時や `Ctrl+C` 中断時に `defer os.Remove` によりディスクから自動消去されます。
4. **Telnet / ネットワーク機器の透過的統合**: Telnet のエスケープ切断（`WithEscapeExit`）や Cisco 特権モード昇格（`enable`）も同一の検索 UI から意識することなく安全に利用可能です。
