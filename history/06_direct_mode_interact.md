# OS非依存・プロンプト不要の対話的接続（DirectMode）の実装履歴

## 1. 概要
`hcm-client` のような対話端末接続クライアントにおいて、FortiGate（`hostname #`）や ATEN（メニュー画面 / プロンプトなし）等のアプライアンス機器へ透過的に接続し、OSごとの `WaitPrompt` を定義することなく `Session.Interact()` へ移行できる「DirectMode（ダイレクト対話モード）」を設計・実装しました。

## 2. 課題と解決策
1. **WaitPrompt 依存によるタイムアウト**:
   - 従来の `DefaultSshLogin` / `DefaultTelnetLogin` は、ログイン完了判定に Linux 想定の `WaitPrompt` を待っていたため、FortiGate や ATEN でタイムアウト（最大600秒）が発生していました。
   - **解決策**: 対話専用の `DirectSshLogin` / `DirectTelnetLogin` を導入し、パスワード送信後または画面出力検知後にプロンプト待ちをバイパスして即座に対話へ移行。
2. **接続初期の遅延・DNS逆引き対策（多段階インターバル判定）**:
   - 接続直後 2 秒以上ホストが無音の場合に鍵認証と誤認しないよう、2 秒タイムアウト後に「何かしら出力があるか」を検査。
   - **無音時**: DNS逆引きやネゴシエーション中と判断し、2秒リトライを最大3回（計8秒）継続。
   - **出力あり時**: 鍵認証成功やメニュー画面表示完了と安全に判定して即座に対話へ移行。
   - **Fast-Path**: 汎用プロンプト記号（`[$#>:%]`）が届いた場合は 0 秒で即時検知。
3. **副作用の排除**:
   - `DirectMode` では `platformRun()`（`export LANG=C` 等）をスキップ。
   - `InteractPreCommand`（`stty echo; stty sane`）や終了時 `exit` 送信を空に設定。

## 3. 変更ファイル
- `src/node/node.go`: `Node` インターフェース、`BaseNode`、`SshNode`、`TelnetNode` に `DirectMode` および `WithDirectMode(true)` を追加。
- `src/session/session.go`: `DirectSshLogin`、`DirectTelnetLogin`、`directLoginLoop`、`Output()` メソッドを実装。
- `src/session/session_wrap.go`: `RunDirectSsh`、`RunDirectTelnet` を追加し、終了時の `exit` 送信をガード。
- `goplur.go`: `RunDirectSsh`、`RunDirectTelnet` をエクスポート。
- `src/node/node_test.go`: `TestNode_DirectMode` を追加。
- `src/session/session_direct_test.go`: FortiGate、パスワード認証、2.5秒遅延ホスト、ATENメニュー画面、無応答タイムアウトのテストを追加。
- `host-credential-manager-go/hcm-client/main.go`: `WithDirectMode(true)` および `WithoutCommands()` を設定。
