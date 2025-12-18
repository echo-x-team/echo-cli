# echo-cli (Go + Bubble Tea)

[English](README.md) | [日本語](readme-jp.md) | [中文](readme-cn.md)

Go と Bubble Tea で作られた Echo Team 向けのコマンドライン + TUI クライアントで、ローカルなエージェント体験を提供します。

## 概要

echo-cli は Echo Team 向けの Go 製 CLI/TUI クライアントです。ターミナル UI に Bubble Tea を採用し、現在は M1 スキャフォールド（CLI エントリ、設定読み込み、基本的な TUI、ツール実行なしのチャット）にフォーカスしています。

## 現在のステータス

- ステージ: **M1 スキャフォールド**（CLI エントリ、設定読み込み、基本的な TUI、ツールなしの Echo Team チャット）。
- Bubble Tea TUI と exec モードは同じセッションパイプラインを共有。ツール実行はサンドボックス統合まではスタブのままです。

## クイックスタート

### TUI を起動

```bash
cd echo-cli
go run ./cmd/echo-cli --prompt "你好"
```

### Exec モード

```bash
go run ./cmd/echo-cli exec --prompt "任务"
```

## 設定と環境

- 環境変数（優先度が最も高い）:
  - `ANTHROPIC_BASE_URL`（例: `https://open.bigmodel.cn/api/anthropic`）
  - `ANTHROPIC_AUTH_TOKEN`（認証トークン）
- 設定ファイル: `~/.echo/config.toml`（`--config <path>` で上書き可能）には次の 2 つのみ:
  - `url = "..."` と `token = "..."`
- それ以外の実行設定（language/timeout 等）は CLI フラグまたは `-c key=value` で指定し、設定ファイルには保存しません。

## CLI（M1+）

- `--config <path>`: 設定ファイルを上書き（デフォルト `~/.echo/config.toml`）。
- `--model <name>`: モデルを上書き。
- `--cd <dir>`: ステータスバーに表示する作業ディレクトリを設定。
- `--prompt "<text>"`: 初期ユーザーメッセージ（位置引数としても利用可能）。
- `ping`: 設定された Anthropic 互換エンドポイントに対して疎通確認を行い、返答テキストを出力。
- `exec <prompt>`: 非対話の JSONL 実行。`--session <id>` / `--resume-last` によるセッション永続化をサポート。
- ツール実行は基本的に自動ですが、危険なコマンドは LLM による審査のうえ人間の承認が必要です。

## AGENTS.md ブートストラップ

- TUI で `/init` を実行すると、エージェントがリポジトリを走査し、agents.md の規約に沿って `AGENTS.md` を下書きします。
- 作業ディレクトリに既に `AGENTS.md` がある場合、このコマンドはファイルを変更せず情報メッセージのみを表示します。

## コード構成

- `cmd/echo-cli`: CLI エントリ。
- `internal/config`: エンドポイント設定の読み込み（url/token）。
- `internal/agent`: エージェントループとモデル抽象化（Anthropic 互換クライアントとストリーミング）。
- `internal/tui`: Bubble Tea UI（トランスクリプト、入力欄、ステータスバー、@ 検索、スラッシュコマンド、セッションピッカー）。
- `internal/tools`: シェルとパッチのヘルパー（直接実行）。
- `internal/search`: `@` ピッカー用のファイル検索。
- `internal/instructions`: システムプロンプト向けの `AGENTS.md` 検出。
- `internal/session`: exec/TUI のセッション保存と復元。

## ロードマップ

- M2: コマンドツール、apply_patch、ファイル検索ピッカー、スラッシュコマンドのオーバーレイ（基本版は実装済み）。
- M3: さらに充実した exec モード（JSONL 同等）、セッションピッカー、AGENTS.md の検出（基本サポート済み）。
- M4: MCP クライアント/サーバー、通知、ZDR。
