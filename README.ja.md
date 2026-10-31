# 🪖 Bulchimbeon（不寝番 / 불침번）

> **あなたの Mac の不寝番。**
> AI コーディングエージェントが**実際に作業している間だけ** Mac をスリープさせず、
> 作業が終わったら「交代します！」と敬礼してスリープに戻すメニューバーアプリです。
>
> **hook もデーモンも常駐する root ヘルパーもなし。アプリひとつで、macOS 13 から。**

韓国語の *불침번（プルチムボン）* は、日本語の **不寝番（ふしんばん）** と同じ漢字。
みんなが眠れるように一人が夜通し見張る当番のことです。ピクセルアートの二等兵が、あなたの Mac の不寝番を務めます。

> 📝 この日本語版は AI による翻訳です。より自然な表現への修正 PR を歓迎します。
> [English](README.md) · [한국어](README.ko.md) · [简体中文](README.zh-CN.md)
> 💡 [Ideas / 문의 / 建议](https://github.com/s3onghyun/bulchimbeon/discussions)
> 🗳️ **どのエージェントと使っていますか？どこでつまずきましたか？** [投票して教えてください](https://github.com/s3onghyun/bulchimbeon/discussions/64)

![Bulchimbeon デモ](docs/media/demo.gif)

## はじめに

インストール: `brew install --cask s3onghyun/tap/bulchimbeon` — または [最新の macOS 用 zip をダウンロード](https://github.com/s3onghyun/bulchimbeon/releases)して `/Applications` に移動します。

**初回起動:** まだ Apple の公証を受けていない ad-hoc 署名のため、macOS が一度ブロックします。

1. Bulchimbeon を開きます。開けないと表示されたら **完了** を押します。
2. **システム設定 → プライバシーとセキュリティ** を開き、**セキュリティ** までスクロールします。
3. *“Bulchimbeon”はブロックされました* の横の **このまま開く** を押して確認します。
   （macOS 15 以降は、右クリック → 開く では回避できません。）

<!-- TODO(mac-capture): add the Gatekeeper screenshot docs/media/gatekeeper-open-anyway.png here -->

通知が **「スクリプトエディタ」** の名前で表示されるのは正常です。公証前は macOS がアプリ自身の通知権限を与えないためです。

**対応エージェント:** Claude Code · Codex · Gemini CLI · Cursor Agent · Copilot CLI · Aider · OpenCode（独自のカスタムエージェントも可）。

### インストール後 1 分チェック

1. エージェントに作業を頼みます。例: Claude Code にテストを実行させる。
2. 数秒以内に、メニューバーの二等兵が懐中電灯をつけます。
3. メニューを開きます。ようこそ表示（**Got it** を押すまで出ます）の下の状態行が **Bulchimbeon — On duty · 0:12 · …**、その下の行に
   **Claude Code working** のように理由が出ます（日本語 UI はまだないため英語表示）。
4. 何も起きない場合は `/Applications/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose` で検出結果を確認し、
   [どこでつまずいたか教えてください](https://github.com/s3onghyun/bulchimbeon/discussions/64)。

macOS が実際に保持しているスリープ防止は `pmset -g assertions` で確認できます。勤務中:

```text
2026-10-31 13:17:17 +0900
Assertion status system-wide:
   BackgroundTask                 0
   ApplePushServiceTask           0
   UserIsActive                   0
   PreventUserIdleDisplaySleep    0
   SoftwareUpdateTask             0
   PreventSystemSleep             0
   ExternalMedia                  0
   PreventUserIdleSystemSleep     1
   NetworkClientActive            0
Listed by owning process:
   pid 2102(Bulchimbeon): [0x002847ba00019690] 13:31:53 PreventUserIdleSystemSleep named: "Bulchimbeon on duty: keeping AI agents awake"
Kernel Assertions: 0x100=MAGICWAKE
   id=592  level=255 0x100=MAGICWAKE creat=  mod= description=en0 owner=IOSkywalkNetworkBSDClient
```

- `2026-10-31 13:17:17 +0900`: コマンドを実行した時刻。
- `Assertion status system-wide`: スリープ防止の種類ごとに、いま保持されている数。
  `PreventUserIdleSystemSleep 1` は何かが Mac のアイドルスリープを止めていることを示します。`PreventUserIdleDisplaySleep 0`
  はディスプレイはスリープしてよいという意味で、既定の勤務モードです。**Keep display on** を選ぶと Bulchimbeon は代わりに
  ディスプレイのスリープ防止を保持するので、この行が 1 になり、下の Bulchimbeon の行の種類もそれに変わります。
- `Listed by owning process`: 誰が保持しているか。`pid 2102(Bulchimbeon)` はアプリ（pid は環境ごとに異なります）、
  `[0x002847ba00019690]` はこのスリープ防止の ID、`13:31:53` は勤務がこれを保持した時刻です。日付がないため前日以前の
  こともあり、ヘッダーの時刻より後に見えることがあります。`PreventUserIdleSystemSleep` は種類
  （**Keep display on** なら `PreventUserIdleDisplaySleep`）、`named: "Bulchimbeon on duty: keeping AI agents awake"` は探すときの固定の名前です。
  Mac を起こしている他のアプリもここに並びます。
- `Kernel Assertions`: macOS 自身の項目（ここでは `en0` のネットワークウェイク）で、Bulchimbeon とは関係なく、環境によって異なります。

macOS 26.6.2、Apple Silicon、Bulchimbeon 0.11.0 でキャプチャ。

待機中（作業中のエージェントなし）は、Bulchimbeon がスリープ防止を解除するので `pid …(Bulchimbeon)` の行は消えます。

<!-- TODO(mac-capture): real idle `pmset -g assertions` output and its line-by-line explanation -->

### よくある質問

**`caffeinate` や Amphetamine ではだめ？** 手動で一定時間 Mac を起こしておくなら良い道具です。Bulchimbeon は AI エージェントが実際に作業している間だけスリープ防止をオン・オフするので、一晩中つけっぱなしになりません。

**何を調べ、何を送る？** hook は使いません。ネットワーク通信は 1 日 1 回の[アップデート確認](README.md#updates)だけで、アプリのバージョン以外は送らず、オフにもできます（`defaults write io.github.s3onghyun.bulchimbeon checkForUpdates -bool false`、Linux は `bulchimbeon config set checkForUpdates false`）。対応エージェントが作業中かを判断するために自分のユーザーのローカルなプロセスのメタデータだけを調べ、プロセスのコマンドラインや環境変数は保存も出力もしません。

**蓋を閉じても起こしておける？** できますが、蓋を閉じるモードをオンにするには管理者の承認が必要です。勤務が終わるかアプリを終了すると、元の蓋のスリープ設定に戻します。

| 状態 | 二等兵 | いつ |
|------|--------|------|
| 💤 就寝 | ヘルメットのまま居眠り、zzz | 見張るものがないとき |
| 👀 待機 | きょろきょろ | エージェントの出勤を待っているとき |
| 🪖 勤務中 | 懐中電灯を持って巡回 | Mac はスリープしません |
| 🫡 交代 | `^ ^` 敬礼 | エージェントが静かになった / 勤務時間終了 |
| 🥵 戦闘休息 | 汗だくで目を閉じてクールダウン | Mac が熱くなりすぎたとき |

**巡回モード**: 二等兵がメニューバーのすぐ下を懐中電灯を持って行き来します。クリックはすべて素通りし、
ポインタが近づくと半透明になって道を譲ります。全画面アプリ（プレゼン・画面共有・動画）の上には現れません。

## なぜ作ったか

エージェントを夜通し動かすために、こうしていました。

```bash
caffeinate -d                 # …止めるのを忘れる
sudo pmset disablesleep 1     # …戻すのを忘れてバッテリー 0%
```

Bulchimbeon は**エージェントが作業している間だけ**起きていて、終わったら自動で交代します。

## 機能

| 機能 | 内容 |
|------|------|
| 🤖 エージェント勤務 | Claude Code・Codex・Gemini CLI・Cursor Agent・Copilot CLI・Aider・OpenCode を検出（独自追加も可）。**開いているだけのセッションは数えず、作業中のときだけ**勤務し、静かになって数分後に交代 |
| 🪖 時間勤務 | 30 分 / 1 時間 / 2 時間 / 4 時間 / 無期限 |
| 勤務形態 | 画面も点けたまま（`caffeinate -d`）・画面は消えても Mac は起きたまま（`caffeinate -i`）・**蓋を閉じても起きたまま**（管理者権限） |
| 🔋 バッテリー撤収 | バッテリー駆動で残量が基準（既定 20%）以下になると撤収 |
| 🌡️ 猛暑注意報 → 🔥 戦闘休息 | Mac が熱くなると警告、危険な温度では勤務を一時停止して冷まし、**冷えたら自動で復帰** |
| ⚠️ 持ち場離脱の検知 | スリープ防止がなぜか外れていたら即座に再投入 |
| 🕵️ 当直の見張り | 蓋を閉じるモード中にアプリが強制終了されても、5 秒以内に元のスリープ設定へ復元 |
| 📋 当直の引き継ぎ | 10 分以上離席して戻ると、その間の勤務内容を報告 |
| 🔁 勤務の継続 | アプリの再起動・アップデート・再起動後も残り時間を引き継ぐ |
| 🖥️ ほかに Mac を起こしているアプリ | 例: 「Vivaldi（動画）」をメニューに表示 |
| 🌐 韓国語 / 英語 / 中国語（簡体字） | 既定はシステム言語。メニューの **언어 / Language / 语言** で切り替え |

## 似たアプリとの比較

同じ課題を解く [Adrafinil](https://github.com/kageroumado/adrafinil) が先行しています。アプローチが逆なので、合う方を選んでください。（2026 年 10 月時点）

| | Bulchimbeon | Adrafinil |
|---|---|---|
| 作業中の判定 | **外から観察**（`caffeinate` 子プロセス・CPU） | **エージェントが通知**（hook）＋観察 |
| インストールで変わるもの | **なし**。エージェントの設定ファイルに触れない | 設定に hook を追加、ユーザーデーモン、root ヘルパー |
| macOS | **13 以降** | 26.4 以降 |
| 蓋を閉じても起動 | 有効化のたびに管理者パスワード、見張りが復元を保証 | インストール時に一度だけ承認 |
| エージェントからの要求 | — | CLI・MCP ツール |
| 配布 | ad-hoc 署名（公証準備中） | 公証済み、Homebrew 公式 cask |

**Adrafinil 向き**: macOS 26.4 以降で、hook による正確な連動やエージェントからの要求が欲しい場合。
**Bulchimbeon 向き**: macOS 13〜15 を使っている、エージェント設定やシステムに何も入れたくない、二等兵に見張ってほしい場合。

## インストール

```bash
brew install --cask s3onghyun/tap/bulchimbeon

# アップデート — Homebrew の自動更新間隔が長いため、先に tap を更新
brew update && brew upgrade --cask bulchimbeon
```

macOS 13 以降、ユニバーサル（Apple Silicon・Intel）。

初回起動: macOS が一度ブロックします。手順は[はじめに](#はじめに)にあります。

## 仕組み（要約）

- スリープ防止は `IOPMAssertion`（`caffeinate` と同じ仕組み）。蓋を閉じるモードは `pmset -a disablesleep 1` を併用
- エージェントは**自分のユーザーのプロセスだけ**を libproc で調べ、実行ファイル名・`argv[0]`・インストールパス・スクリプトパスで判定（部分一致はしない）
- 「作業中」の判定: Claude Code が作業中にだけ自ら起動する `caffeinate` 子プロセス、またはプロセスツリーの CPU 使用率（60 秒平均）
- 判定ロジックはシステム呼び出しのない状態機械（`DutyEngine`）にあり、偽の時計でシナリオテスト済み

詳細: [English README](README.md) · [設計ドキュメント（韓国語）](docs/design.md)

## セキュリティ

管理者権限で `pmset` を実行し、他プロセスの情報を読むアプリなので、脅威モデルを別にまとめています: [SECURITY.md](SECURITY.md)

- ネットワーク通信は 1 日 1 回のアップデート確認のみ（バージョンだけを送信、オフにできる）、サードパーティ依存ゼロ
- 他プロセスの引数・環境変数は保存も表示もしない。AI の会話ログは読まない
- リリースは GitHub Actions でのみビルドし、SHA-256 チェックサムとビルド来歴証明（attestation）付き

## コントリビュート

バグ報告、エージェント追加のリクエスト、セリフやスプライトのアイデア、そして**この翻訳の改善**も歓迎します: [CONTRIBUTING.md](CONTRIBUTING.md)

## ライセンス

MIT
