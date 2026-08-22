# template-base

新規リポジトリ作成時のベーステンプレート。

## 使い方

1. このリポジトリをベースに新規リポジトリを作成
2. `.drone.yml` の `steps:` に `build` / `deploy` ステップを追加
3. Drone CI のシークレットを各リポジトリで設定

## CI/CD

Drone CI（drone-runner-kube）を使用。このテンプレートは no-op パイプラインのみ定義済み。各リポジトリでステップを追加する。

## AI レビュー

Claude と Codex が PR を自動レビューする。

| Bot | トリガー | Approve |
|---|---|---|
| Claude | `@claude` コメント（org メンバー・コラボレーター限定） | 問題なければ自動 Approve |
| Codex | PR open / `@codex review` コメント | 問題なければ自動 Approve（bridge 経由） |

- レビューコメントはすべて日本語
- 設定は `CLAUDE.md`（Claude 用）・`AGENTS.md`（Codex 用）を参照
