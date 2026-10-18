# 🎙️ mogkoe（もじこえ）

複数のDiscord BOTを同時に運用し、VOICEVOX Engineでテキストを読み上げるGo製のDiscord読み上げBOTです。

Discordのスラッシュコマンド、VOICEVOX、Docker Compose、Redisによる複数BOT排他制御、現行Voice GatewayのAEAD音声暗号化方式に対応しています。

## ✨ 主な機能

- 🤖 `BOT_1_TOKEN`〜`BOT_10_TOKEN`による複数BOT同時起動
- 📡 BOTごとのスラッシュコマンド自動登録
- 🎙️ ボイスチャンネルへの参加・退出
- 🔊 VOICEVOXによる日本語読み上げ
- 🗣️ サーバーごとの標準話者カスタマイズ
- 🧠 文章内容に応じた話者自動選定
- 📣 サーバーごとの自動読み上げチャンネル指定
- ⏯️ 読み上げの一時停止・再開
- 🧹 待機中の読み上げキュー消去
- 💾 音声キャッシュによる再生成削減
- 🛡️ Redisによる複数BOTのリーダー選出と二重読み上げ防止
- 🩺 Gateway遅延監視と異常BOTの自動フェイルオーバー
- 📡 `/help｜遅延ms｜N号機`形式の視聴中ステータス
- 🖼️ Discord Embedによる成功・エラー・状態表示
- 🐳 BOT・VOICEVOX・RedisのDocker Compose一括起動

## 📋 動作要件

- Docker Engine
- Docker Compose v2
- Discord Developer PortalのBOTアプリケーション
- BOTを追加するDiscordサーバー
- 音声読み上げを利用するボイスチャンネル

ローカルでGoテストやビルドを行う場合はGo 1.22以降が必要です。Docker運用だけであれば、ホスト側にGoをインストールする必要はありません。

## 🔐 Discord Developer Portalの設定

### BOT作成

Discord Developer PortalでApplicationを作成し、Botを追加します。BOTトークンは外部へ公開しないでください。

### Privileged Gateway Intent

自動読み上げではメッセージ本文を取得するため、Bot設定の**Privileged Gateway Intents**で次を有効化します。

- **Message Content Intent**

コード側では次のIntentを使用します。

- `GUILDS`
- `GUILD_VOICE_STATES`
- `GUILD_MESSAGES`
- `MESSAGE_CONTENT`

### 招待URLのスコープと権限

BOTを招待するときは次のスコープを使用します。

- `bot`
- `applications.commands`

最低限、利用するサーバーとチャンネルで次の権限を付与してください。

- チャンネルを見る
- メッセージを読む
- メッセージ履歴を読む
- メッセージを送信
- 埋め込みリンク
- 接続
- 発言

自動読み上げ対象のテキストチャンネルと、BOTが参加するボイスチャンネルの権限を分けて設定することもできます。

## 🚀 導入手順

### 1. プロジェクトを取得

```sh
git clone https://github.com/kawaewanko/mogkoe.git
cd mogkoe
```

ZIPで取得した場合は、`mogkoe`ディレクトリへ移動して作業します。

### 2. 環境変数ファイルを作成

```sh
cp .env.example .env
```

最低限、`BOT_1_TOKEN`を設定します。

```dotenv
BOT_1_TOKEN=DiscordのBOTトークン
BOT_2_TOKEN=
DISCORD_GUILD_ID=テスト用サーバーID
VOICEVOX_URL=http://voicevox:50021
REDIS_URL=redis://redis:6379/0
VOICE_DEFAULT_SPEAKER=3
AUDIO_CACHE_TTL=3600
LATENCY_POLL_INTERVAL_SECONDS=15
LATENCY_ALERT_THRESHOLD_MS=1500
LATENCY_FAILURE_LIMIT=3
LATENCY_RECOVERY_LIMIT=2
LOG_LEVEL=info
```

### 3. スラッシュコマンド登録先を設定

`DISCORD_GUILD_ID`を指定すると、そのサーバーへギルドコマンドとして登録します。開発中の確認に適しており、反映は比較的早いです。

```dotenv
DISCORD_GUILD_ID=123456789012345678
```

空欄にするとグローバルコマンドとして登録されます。グローバルコマンドは利用可能になるまで時間がかかることがあります。

### 4. 起動

```sh
docker compose up -d --build
```

ログを確認します。

```sh
docker compose logs -f mogkoe
```

次の3サービスが起動します。

- `mogkoe`: Discord BOT本体
- `voicevox`: VOICEVOX Engine
- `redis`: 設定保存・BOT間排他制御・ヘルス情報共有

### 5. 初回操作

1. Discordでボイスチャンネルへ参加
2. `/join`を実行
3. 必要に応じて`/channel channel:#読み上げチャンネル`を実行
4. 必要に応じて`/speakers`で話者一覧を確認
5. `/speaker speaker_id:3`などで標準話者を設定
6. テキストチャンネルへ文章を投稿
7. `/status`で接続・話者・チャンネル・キュー状態を確認

## 🧰 スラッシュコマンド

| コマンド | 説明 |
| --- | --- |
| `/join` | 実行者が参加中のボイスチャンネルへBOTを参加させます |
| `/leave` | BOTをボイスチャンネルから退出させます |
| `/speak text:...` | サーバー標準話者で文章を読み上げます |
| `/voice text:...` | 文章に合う話者を自動選定して読み上げます |
| `/channel channel:#チャンネル` | 自動読み上げの対象チャンネルを指定します |
| `/channel-off` | 自動読み上げチャンネルの指定を解除します |
| `/speaker speaker_id:...` | サーバー標準話者をVOICEVOXスタイルIDで設定します |
| `/speaker-reset` | 標準話者を`VOICE_DEFAULT_SPEAKER`の値へ戻します |
| `/speakers` | 利用可能なVOICEVOX話者とスタイルIDを表示します |
| `/pause` | 読み上げを一時停止します |
| `/resume` | 一時停止した読み上げを再開します |
| `/clear` | 待機中の読み上げキューを消去します |
| `/status` | BOT、音声エンジン、話者、チャンネル、キューの状態を表示します |

`/speak`と自動読み上げはサーバー標準話者を使用します。`/voice`は文章ルールによる自動選定を優先します。

## 🗣️ 話者設定

`/speakers`でVOICEVOX Engineが提供する話者名とスタイルIDを取得できます。設定には話者名ではなくスタイルIDを指定します。

```text
/speaker speaker_id:3
```

設定はRedisへ保存されるため、BOTを再起動しても維持されます。`/speaker-reset`で環境変数`VOICE_DEFAULT_SPEAKER`の値へ戻せます。

初期の自動選定ルールは次のとおりです。

- 疑問符・感嘆符を含む文章 → スタイルID `8`
- 「おはよう」「こんばんは」を含む文章 → スタイルID `1`
- 「ありがとう」「よろしく」を含む文章 → スタイルID `3`
- その他 → サーバー標準話者

自動選定ルールを変更する場合は`internal/discord/handlers.go`の`autoSpeaker`を編集します。

## 📣 読み上げチャンネル指定

```text
/channel channel:#general
```

指定したサーバーのテキストチャンネルだけを自動読み上げ対象にします。設定はRedisへ保存され、複数BOTと再起動後のBOTで共有されます。

```text
/channel-off
```

チャンネル指定を解除し、サーバー内の全テキストチャンネルを自動読み上げ対象へ戻します。

## ⏯️ キュー操作

`/pause`は再生中の音声をフレーム境界で一時停止し、待機中のキューを保持します。`/resume`で続きから再生します。

`/clear`は待機中の音声だけを消去します。実際に再生中の文章は継続します。

## 🛡️ 複数BOT運用

`BOT_1_TOKEN`、`BOT_2_TOKEN`のように複数トークンを設定すると、複数BOTが同じプロセスで起動します。

Redisは次の用途で使われます。

- ギルド単位のリーダー選出
- メッセージID単位の重複読み上げ防止
- 読み上げチャンネル設定の共有
- 標準話者設定の共有
- BOTごとのヘルス状態共有

同じサーバーで複数BOTが同時に動作しても、同一メッセージを複数回読み上げない設計です。Redisが停止した場合は重複を避けるため、自動読み上げを安全側で停止します。

## 🩺 遅延監視とフェイルオーバー

各BOTは視聴中ステータスを次の形式で表示します。

```text
/help｜42ms｜1号機
```

遅延値はDiscord GatewayのHeartbeat遅延です。`LATENCY_POLL_INTERVAL_SECONDS`の間隔で更新し、初期値は15秒です。

```dotenv
LATENCY_POLL_INTERVAL_SECONDS=15
LATENCY_ALERT_THRESHOLD_MS=1500
LATENCY_FAILURE_LIMIT=3
LATENCY_RECOVERY_LIMIT=2
```

遅延が`LATENCY_ALERT_THRESHOLD_MS`を超える、またはGateway切断状態が`LATENCY_FAILURE_LIMIT`回続くと、BOTを不健康状態としてRedisへ登録します。現在のリーダーが不健康になった場合は、正常な別BOTが自動的に読み上げ処理を引き継ぎます。

正常な状態が`LATENCY_RECOVERY_LIMIT`回続くと、BOTは自動的に復帰します。ポーリング間隔は5秒未満には設定されません。

## 🔐 Discord音声暗号化

音声接続はプロジェクト内の`third_party/discordgo`を使用します。Voice Gatewayが通知する暗号化方式を確認し、次の優先順位で選択します。

1. `aead_aes256_gcm_rtpsize`
2. `aead_xchacha20_poly1305_rtpsize`

従来の`xsalsa20_poly1305`は選択しません。Opus音声は48kHz・ステレオ・20msフレームで送信します。

## 🗂️ ディレクトリ構成

```text
mogkoe/
├── assets/                  # BOTアイコン・ヘッダー
├── cmd/mogkoe/              # エントリーポイント
├── internal/config/         # 環境変数の読み込み
├── internal/coordination/   # Redis共有設定・排他制御
├── internal/discord/        # Discord接続・コマンド・キュー
├── internal/voicevox/       # VOICEVOX API・音声キャッシュ
├── third_party/discordgo/   # AEAD対応discordgo
├── Dockerfile               # マルチステージビルド
└── docker-compose.yml       # BOT・VOICEVOX・Redis
```

## 🛠️ 開発・検証コマンド

```sh
gofmt -w cmd internal
go test ./...
go vet ./...
go mod tidy
```

ビルド確認:

```sh
go build ./cmd/mogkoe
```

Docker Composeの再ビルド:

```sh
docker compose build --no-cache mogkoe
docker compose up -d
```

停止:

```sh
docker compose down
```

データボリュームも削除して初期化する場合は、設定とキャッシュが消えるため注意してください。

```sh
docker compose down -v
```

## 🧯 トラブルシューティング

### スラッシュコマンドが表示されない

`DISCORD_GUILD_ID`を確認し、ギルドコマンドとして起動してください。グローバル登録は反映まで時間がかかる場合があります。BOT招待URLに`applications.commands`が含まれていることも確認します。

### メッセージを読み上げない

Message Content Intentが有効か確認します。次に`/join`、`/channel`の設定、BOTのメッセージ閲覧権限、RedisとVOICEVOXの稼働状態を確認してください。

### 音声チャンネルへ参加できない

BOTに`View Channel`、`Connect`、`Speak`権限があるか確認します。`docker compose logs -f mogkoe`でVoice Gatewayのエラーを確認します。

### `/speaker`で話者を設定できない

`/speakers`で現在のVOICEVOX Engineが提供するスタイルIDを確認してください。VOICEVOXのバージョンやインストール済みモデルによって利用可能な話者が異なる場合があります。

### Redis停止後に自動読み上げが再開しない

Redisを復旧してから新しいメッセージを投稿してください。設定はRedisボリュームに保存されます。必要なら`docker compose logs redis`で確認します。

## 💾 データとセキュリティ

- `.env`、BOTトークン、Redisの認証情報をGitへ追加しないでください。
- `audio-cache`には生成音声キャッシュが保存されます。
- `redis-data`にはチャンネル設定、話者設定、Redisの永続データが保存されます。
- DiscordのMessage Content Intentを使うため、サーバー利用者へ読み上げの運用方針を案内してください。
- VOICEVOXおよび各キャラクターの利用規約・商用利用条件を確認してください。
- Discord APIのレート制限を守って運用してください。

## 🎨 ブランドアセット

`assets/`には、丸みのある「G／9」形状とヘッドマイクを組み合わせたmogkoeのロゴアセットを収録しています。

- `assets/mogkoe-icon.png`: Discord BOTアイコン向け
- `assets/mogkoe-header.png`: Discordプロフィールヘッダー向け

## 🔗 リポジトリとライセンス

- GitHub: <https://github.com/kawaewanko/mogkoe>
- ライセンス: MIT License（[`LICENSE`](LICENSE)）
