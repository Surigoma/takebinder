# アーキテクチャ

[English](architecture.md)

## 1. システム構成

```text
Electron Renderer
  └─ 画面、設定、進捗表示
       │ IPC（許可済みAPIのみ）
Electron Main
  └─ ウィンドウ、フォルダ選択、Goプロセス管理
       │ JSON Lines over stdin/stdout
Go Backend
  ├─ ファイル走査・グループ判定
  ├─ ジョブキュー・並列数制御
  ├─ コピー・ハッシュ・レポート生成
  ├─ メディアツール実行・キャンセル
  └─ 出力検証
       ├─ FFmpeg（ストリームコピー結合）
       ├─ ffprobe（ストリーム検査）
       └─ GPMF parser（テレメトリ検証候補）
```

## 2. Electron

- TypeScriptを使用する。
- RendererはReact、Material UI、i18next、react-i18nextを使用し、Viteでブラウザ向けES moduleへbundleする。
- Electron MainとpreloadはTypeScript CompilerでNode.js向けCommonJSへ分離して出力する。
- RendererからNode.js APIへ直接アクセスさせない。
- `contextIsolation`を有効、`nodeIntegration`を無効にする。
- preloadで必要最小限のAPIだけを公開する。
- ファイル処理ロジックをRendererへ置かない。
- Rendererからの実行要求ではグループIDだけを採用し、ファイルパスと出力先はGoバックエンドが保持する直前の解析結果から復元する。未解析、重複、要確認のグループは拒否する。

## 3. Goバックエンド

- 1つの実行ファイルとして各OS／CPU向けにビルドする。
- Electronとは標準入力・標準出力上のJSON Linesで通信する。
- 標準出力には機械可読イベントのみを出し、診断ログは標準エラーへ出す。
- コンテキストキャンセルによりジョブと子プロセスを停止できるようにする。
- 同時実行数をセマフォで制限する。
- パスはシェル文字列へ連結せず、引数配列として外部プロセスへ渡す。
- 走査時はffprobeで映像、GoPro固有タグ、`gpmd`を確認する。Media ID、Capture ID、Chapterタグが取得できる場合はグループ判定へ優先利用し、取得できない場合だけGoProファイル名規則へフォールバックして根拠と信頼度を返す。
- DJIの試験検出はPocket機種タグまたはDJIメタデータ・ファイル名候補を用い、録画グループ化せず独立したコピー単位を返す。未知のDJIメタデータを含めMP4全体をコピーし、結合は無効とする。[対応状況](osmo-pocket-support.md)を参照する。
- `original` 保存は入力を読み取り専用で開き、SHA-256照合済みの一時ファイルを同じ出力ファイルシステム上で完成名へ確定する。開始前に完成動画と元動画コピーの推定容量をOS APIで確認し、不足時は `E_DISK_FULL` とする。
- 結合出力には先頭チャプターの `creation_time` を設定し、ffprobeで一致を検証してから完成名へ確定する。完成後にファイル更新日時を設定し、Windowsでは作成日時も設定する。未対応OSやファイルシステムでの日時設定失敗は警告とレポートへ残し、検証済みメディアを破損扱いにはしない。
- 既定出力名と入力からの相対サブフォルダは走査時にGoバックエンドが確定し、実行時は保存済みの走査計画から復元する。日付フォルダ併用時は `日付/入力相対サブフォルダ/動画` とする。

## 4. ネイティブメディアツール

ElectronとGoのコードは各OS／CPU向けにビルドする。FFmpegとffprobeはアプリへ同梱せず、ユーザーが画面から明示的に取得した固定版の単体バイナリをアプリ管理領域へ保存する。

配布物は対象OSのネイティブ環境で `package:windows`、`package:macos`、`package:linux` の各Taskを実行して生成し、CIのOSマトリクスで各Taskを検証する。

採用ツールは、実GoProサンプルを使った技術検証で次を満たしたものに確定する。

- `gpmd`トラックを欠落させず連結できる
- タイムスタンプとサンプルテーブルを正しく再構成できる
- H.264およびH.265を再エンコードしない
- 商用配布を想定したライセンス条件を満たす

複数チャプターはFFmpegのconcat demuxerへ順番どおりに渡し、映像、音声、`gpmd`を明示的にmapして `-c copy` で多重化する。コマンドの成功だけでは完了扱いにせず、ffprobeで入力と出力の映像・音声仕様、合計時間、全パケットのSHA-256・順序・サイズを照合する。`gpmd`はさらに動画全体を覆う時間範囲と、入力に存在した主要キー（`GYRO`、`ACCL`、`GPS5`）の保持を検証する。実GoProサンプルでは、映像・音声・`gpmd`の全パケットpayload一致と主要GPMFキーの保持を確認済みである。

## 5. バックエンド通信仕様

### 5.1 コマンド

| コマンド | 用途 |
| --- | --- |
| `scan` | 入力フォルダを解析し、撮影グループ候補を返す |
| `run` | 確認済みの処理計画を実行する |
| `cancel` | 実行中の処理を停止する |
| `status` | バックエンドと同梱ツールの状態を返す |
| `install-tools` | 固定manifestに含まれる外部ツールを取得・検証する |

### 5.2 イベント

| イベント | 用途 |
| --- | --- |
| `scan.progress` | ファイル走査の進捗 |
| `scan.completed` | グループ化結果 |
| `job.started` | グループ処理開始 |
| `job.progress` | グループ処理進捗 |
| `job.warning` | 継続可能な警告 |
| `job.completed` | グループ処理成功 |
| `job.failed` | グループ処理失敗 |
| `run.completed` | 全体処理終了 |
| `tools.install.progress` | ツール取得のバイト進捗 |
| `tools.install.completed` | FFmpegとffprobeの取得結果 |

各メッセージは `protocolVersion`、`requestId`、`type`、`payload` を持つ。

## 6. ソフトウェア・サプライチェーン保護

### 6.1 Yarn依存

- Yarn 4.17.1を `packageManager` で固定し、Corepack経由でCIと開発環境を揃える。
- `yarn.lock` を必ずコミットし、CIとリリースビルドは `corepack yarn install --immutable` を使う。
- 直接依存は完全なバージョンへ固定し、推移依存はlockfileで固定する。
- 依存追加は必要性、保守状況、公開者、リポジトリ、ライセンス、既知脆弱性、install-timeスクリプトの有無をレビューする。
- Git URL、任意tarball、ローカルパスからの依存は原則禁止し、npmレジストリ上の固定バージョンを使用する。
- CIでlockfileとの差分や不整合が生じた場合は失敗させる。依存更新だけを行うPRは、アプリ変更と分離する。
- CIや手順書で、未固定パッケージを取得する `yarn dlx` を使わない。タスクは `Taskfile.yml` に固定する。
- 新規公開直後の依存を避けるため `npmMinimalAgeGate` を7日へ設定する。

### 6.2 install-timeスクリプト

- `.yarnrc.yml` の `enableScripts: false` で依存のinstall-timeスクリプトを無効化する。
- lifecycle処理が必要な依存は、内容とバージョンを確認した明示Taskとして実行する。Electron 43は `task install:electron` だけで取得する。
- 許可追加時はスクリプト内容、ダウンロード先、生成物を確認する。包括的なスクリプト許可は禁止する。

### 6.3 脆弱性と更新

- PRで `yarn npm audit --recursive` と依存差分レビューを実行する。high/criticalは、影響調査と期限付きの例外記録がない限りマージしない。
- `yarn up` による無検証の一括更新は禁止する。
- ElectronはChromium/Node.jsを含むため、サポート中のバージョンを使い、セキュリティ更新を優先する。
- 自動更新PRは許可するが、自動マージしない。lockfile、install-timeスクリプト、テスト結果をレビューする。

### 6.4 CIとリリース

- TaskはCIでv3.51.1へ固定し、`Taskfile.yml` を開発環境とCIの共通入口にする。
- GitHub Actionsの第三者Actionは完全なcommit SHAへ固定する。
- workflowの `permissions` はジョブごとに最小化し、通常のテストは読み取り権限とする。
- 外部コントリビューターのPRでは署名鍵、公開トークン、その他のリリース秘密へアクセスさせない。
- リリースは保護されたタグまたは承認付き環境から、クリーンなcheckoutとimmutable installで再現する。
- コード署名鍵は依存インストールやテスト工程へ渡さず、検証済み成果物の署名工程だけへ渡す。
- SBOMは追加依存を取得せず、固定Yarnの `yarn info -A -R --json` からCycloneDX 1.6 JSONを生成する。配布時にlockfile由来の依存グラフと再照合し、配布物へ含める。

### 6.5 Goと同梱バイナリ

- `go.mod` と `go.sum` をコミットし、Go依存の変更も独立PRでレビューする。
- FFmpeg、ffprobeなどの取得物はバージョンと取得元を固定し、公開済みチェックサムおよびリポジトリ管理のSHA-256で検証する。
- 出所、バージョン、ライセンス、チェックサムをリリースマニフェストへ記録する。
- ビルド時にネットワークから「最新版」を取得しない。

### 6.6 参考資料

- [Yarn installation](https://yarnpkg.com/getting-started/install)
- [Yarn Corepack](https://yarnpkg.com/corepack)
- [Task installation](https://taskfile.dev/docs/installation)
- [Electron Security](https://www.electronjs.org/docs/latest/tutorial/security)
- [GitHub Actions hardening](https://docs.github.com/en/code-security/tutorials/secure-your-organization/protect-against-threats)

### 6.7 ネイティブツール取得

- 取得URL、バージョン、ファイル名、サイズ、SHA-256はGoバイナリへ固定する。サーバー上のリモートmanifestで実行時に差し替えない。
- HTTPSのみ許可し、初回URLとリダイレクト先のホストを許可リストで検査する。
- ダウンロードは上限サイズ付き一時ファイルへ行い、サイズとSHA-256が一致した場合だけアプリ管理領域へ確定する。
- FFmpegとffprobeはShaka Projectの固定リリースにある単体バイナリを取得し、`-version` が成功した後に利用可能とする。インストーラーは実行しない。
- 現在の対象はWindows x64、macOS x64/arm64、Linux x64/arm64である。
