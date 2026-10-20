# Osmo Pocket / DJI対応状況

[English](osmo-pocket-support.md)

## 対象範囲

Osmo Pocket各世代とDJI形式の名前を持つMP4は試験対応として検出する。TakeBinderは、実カメラ素材で検証した保守的な条件を満たす**Osmo Pocket 4**長時間撮影の分割を、ロスレスで結合できる。その他のPocket世代、名前を変更したファイル、汎用的なDJI候補は、ファイル単位でバイトコピーする。

## 検証結果（2026-10-18）

1時間52分の同一撮影から得た未編集のPocket 4素材3チャプターを調査・結合した。内訳は約16 GiBの分割2本と残り1本である。各入力にはHEVC、2本のAAC、`djmd`、`dbgi`、`tmcd`、添付MJPEGトラックが含まれる。フォーマットのencoderは`DJI OsmoPocket4`であり、連番、ファイル名時刻、コンテナ撮影日時、タイムコードは連続している。Gyroflow `mp4-merge` 0.1.11によって、全7トラックを持つ想定時間どおりの43.7 GB出力を作成できた。TakeBinderは、HEVC、2本のAAC、`djmd`、`dbgi`について、連結された全パケットのpayload、順序、個数、総バイト数を比較する。mp4-mergeは先頭チャプターの添付MJPEGサムネイルを保持する。`tmcd`については、TakeBinderが先頭チャプターの4バイトSMPTE開始payloadを保持し、章間タイムコードの連続性を確認したうえで、そのMP4サンプル時間を結合映像全体へ延長する。

この検証結果が保証するのは、提供されたPocket 4サンプルだけである。その他のファームウェア、撮影モード、解像度、フレームレート、HDRプロファイル、命名設定、Pocket世代は対応済みとみなさない。

調査資料:

- [DJI: Osmo Action / Osmo Pocketシリーズの動画分割仕様](https://repair.dji.com/help/content?customId=01700006948&lang=en&paperDocType=ARTICLE&re=US&spaceId=17)
- [DJI: Pocket 3ユーザーマニュアル](https://dl.djicdn.com/downloads/DJI_Osmo_Pocket_3/UM/20250826/DJI_Osmo_Pocket_3_User_Manual_v1.0_en.pdf)
- [ExifTool DJI parser](https://github.com/exiftool/exiftool/blob/master/lib/Image/ExifTool/DJI.pm)
- [Gyroflow mp4-merge](https://github.com/gyroflow/mp4-merge)（MITまたはApache-2.0）

## 検出・グループ化・安全性の契約

1. ffprobeが映像ストリームを検出し、`model`、`camera_model`、QuickTime model、`encoder`のいずれかからPocketの正確な機種を特定できなければならない。実Pocket 4サンプルでは`encoder=DJI OsmoPocket4`を使用する。
2. 自動結合には、正確な名前`DJI_YYYYMMDDhhmmss_NNNN_D.MP4`、同一ディレクトリ、連続する番号、同一の全ストリーム構成、直前ファイルが15 GiB以上、次のファイル名時刻が直前メディアの終了時刻から3秒以内、SMPTEドロップフレームタイムコードが2フレーム以内で連続することをすべて要求する。1条件でも満たさなければ独立したコピー単位とし、途切れを推測で結合しない。
3. 結合単位には根拠`DJI Pocket 4 split continuity`と高信頼を設定する。汎用的なCapture/Chapterタグが存在しても、一般的な`DJI_`形式の名前は低信頼の`DJI file copy`単位とする。
4. `mp4-merge`はシェルを経由せず、引数配列を使って直接起動する。メディアをデコード・エンコードせず、MP4サンプルテーブルを再構築する。`backend/tools.go`には、バージョン0.1.11の単体バイナリ、URL、サイズ、SHA-256を固定する。インストーラーは実行しない。
5. 出力は一時ファイルへ書き込む。ffprobeで全ストリーム構成、合計時間、先頭チャプターの撮影日時を確認しなければならない。HEVC、2本のAAC、`djmd`、`dbgi`は、SHA-256パケットフィンガープリントが連結した入力と一致しなければならない。MJPEGと`tmcd`のpayloadは先頭チャプターと一致し、延長した`tmcd`時間は結合映像全体を覆わなければならない。不一致があれば`E_DJI_VERIFY`を返して一時ファイルを削除し、完成名のファイルは作成しない。
6. 単独ファイルは従来どおりアトミックなバイトコピーを行う。入力は変更・移動・削除せず、既存出力を上書きしない。
7. DJIグループの結合に成功した場合は、既存プロトコルのフィールドに`gpmfVerification: all-tracks-verified`を設定する。これはDJIのメディア／データトラックと連続する`tmcd`の意味を検証したことを表し、DJIにGoPro GPMFが含まれるという意味ではない。

## 今後の検証

対応規則を広げる前に、各世代、ファームウェア、撮影モードについて、連続する分割と隣接する別撮影の素材を収集する。対象とする編集ソフトでの再生互換性も確認する。未知または検証不能なメタデータがある場合、結合出力を完了扱いしない。
