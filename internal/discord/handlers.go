package discord

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/hraban/opus"
	"github.com/kawaewanko/mogkoe/internal/voicevox"
)

const (
	colorSuccess = 0x57D38C
	colorInfo    = 0x5865F2
	colorWarning = 0xF0B232
	colorError   = 0xED4245
)

func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	switch i.ApplicationCommandData().Name {
	case "help":
		b.help(s, i)
	case "join":
		b.join(s, i)
	case "leave":
		b.leave(s, i)
	case "speak", "voice":
		b.speak(s, i, i.ApplicationCommandData().Name == "voice")
	case "channel":
		b.setChannel(s, i)
	case "channel-off":
		b.clearChannel(s, i)
	case "speaker":
		b.setSpeaker(s, i)
	case "speaker-reset":
		b.resetSpeaker(s, i)
	case "speakers":
		b.listSpeakers(s, i)
	case "pause":
		b.pause(s, i)
	case "resume":
		b.resume(s, i)
	case "clear":
		b.clearQueue(s, i)
	case "status":
		b.status(s, i)
	}
}

func (b *Bot) help(s *discordgo.Session, i *discordgo.InteractionCreate) {
	fields := []*discordgo.MessageEmbedField{
		{Name: "/join・/leave", Value: "ボイスチャンネルへの参加・退出", Inline: false},
		{Name: "/speak・/voice", Value: "標準話者・自動選定で文章を読み上げ", Inline: false},
		{Name: "/channel・/channel-off", Value: "自動読み上げチャンネルの指定・解除", Inline: false},
		{Name: "/speaker・/speaker-reset・/speakers", Value: "標準話者の設定・リセット・話者一覧の表示", Inline: false},
		{Name: "/pause・/resume・/clear", Value: "読み上げの一時停止・再開・キュー消去", Inline: false},
		{Name: "/status", Value: "BOT、音声エンジン、話者、キューの状態を表示", Inline: false},
	}
	b.replyEmbedFields(s, i, "📖 mogkoe ヘルプ", "利用できるスラッシュコマンドの一覧です。詳しい引数はDiscordの入力欄で確認できます。", colorInfo, fields, false)
}

func (b *Bot) onMessage(_ *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Author == nil || message.Author.Bot || message.GuildID == "" || message.Content == "" {
		return
	}
	value, ok := b.rooms.Load(message.GuildID)
	if !ok {
		return
	}
	channelContext, channelCancel := context.WithTimeout(context.Background(), 2*time.Second)
	textChannelID, channelErr := b.coordinator.GetTextChannel(channelContext, message.GuildID)
	channelCancel()
	if channelErr != nil || (textChannelID != "" && textChannelID != message.ChannelID) {
		return
	}
	claimContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	claimed, err := b.coordinator.ClaimMessage(claimContext, message.GuildID, message.ID)
	cancel()
	if err != nil || !claimed {
		return
	}
	room := value.(*room)
	go b.enqueue(room, message.Content, autoSpeaker(message.Content))
}

func (b *Bot) join(s *discordgo.Session, i *discordgo.InteractionCreate) {
	voiceState, err := s.State.VoiceState(i.GuildID, i.Member.User.ID)
	if err != nil || voiceState == nil || voiceState.ChannelID == "" {
		b.replyError(s, i, "参加できません", "先にボイスチャンネルへ参加してください。")
		return
	}

	voice, err := s.ChannelVoiceJoin(i.GuildID, voiceState.ChannelID, false, false)
	if err != nil {
		b.replyError(s, i, "参加できません", err.Error())
		return
	}

	encoder, err := opus.NewEncoder(48000, 2, opus.AppAudio)
	if err != nil {
		_ = voice.Disconnect()
		b.replyError(s, i, "音声機能を初期化できません", err.Error())
		return
	}

	newRoom := &room{voice: voice, encoder: encoder}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	textChannelID, channelErr := b.coordinator.GetTextChannel(ctx, i.GuildID)
	cancel()
	if channelErr != nil {
		b.log.Error("読み上げチャンネル設定の取得に失敗しました", "guild_id", i.GuildID, "error", channelErr)
	} else {
		newRoom.textChannelID = textChannelID
	}
	b.rooms.Store(i.GuildID, newRoom)
	description := fmt.Sprintf("`%s` で読み上げを開始できます。", voiceState.ChannelID)
	if textChannelID != "" {
		description += fmt.Sprintf("\n自動読み上げ: <#%s>", textChannelID)
	}
	b.replyEmbed(s, i, "🎙️ ボイスチャンネルに参加しました", description, colorSuccess, false)
}

func (b *Bot) leave(s *discordgo.Session, i *discordgo.InteractionCreate) {
	value, ok := b.rooms.LoadAndDelete(i.GuildID)
	if !ok {
		b.replyEmbed(s, i, "ℹ️ 接続していません", "mogkoeはこのサーバーのボイスチャンネルに接続していません。", colorInfo, true)
		return
	}

	_ = value.(*room).voice.Disconnect()
	b.replyEmbed(s, i, "👋 ボイスチャンネルから退出しました", "読み上げキューをクリアしました。", colorSuccess, false)
}

func (b *Bot) speak(s *discordgo.Session, i *discordgo.InteractionCreate, auto bool) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		b.replyError(s, i, "文章がありません", "読み上げる文章を指定してください。")
		return
	}

	text := strings.TrimSpace(options[0].StringValue())
	if text == "" || len([]rune(text)) > 200 {
		b.replyError(s, i, "文章を確認してください", "文章は1〜200文字で指定してください。")
		return
	}

	value, ok := b.rooms.Load(i.GuildID)
	if !ok {
		b.replyError(s, i, "接続されていません", "先に `/join` を実行してください。")
		return
	}

	speaker := b.engineDefault(i.GuildID)
	mode := "標準話者"
	if auto {
		speaker = autoSpeaker(text)
		mode = "自動選定"
	}

	room := value.(*room)
	queueSize := room.queueLength()
	b.replyEmbed(s, i, "🔊 読み上げキューに追加しました", fmt.Sprintf("**方式:** %s\n**文字数:** %d\n**待ち件数:** %d", mode, len([]rune(text)), queueSize), colorInfo, false)
	go b.enqueue(room, text, speaker)
}

func (b *Bot) setChannel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		b.replyError(s, i, "チャンネルがありません", "自動読み上げに使うテキストチャンネルを指定してください。")
		return
	}
	channel := options[0].ChannelValue(s)
	if channel == nil || channel.GuildID != i.GuildID || (channel.Type != discordgo.ChannelTypeGuildText && channel.Type != discordgo.ChannelTypeGuildNews) {
		b.replyError(s, i, "チャンネルを確認してください", "このサーバーのテキストチャンネルを指定してください。")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := b.coordinator.SetTextChannel(ctx, i.GuildID, channel.ID)
	cancel()
	if err != nil {
		b.replyError(s, i, "チャンネルを設定できません", err.Error())
		return
	}
	if value, ok := b.rooms.Load(i.GuildID); ok {
		r := value.(*room)
		r.mu.Lock()
		r.textChannelID = channel.ID
		r.mu.Unlock()
	}
	b.replyEmbed(s, i, "📣 読み上げチャンネルを設定しました", fmt.Sprintf("自動読み上げの対象を <#%s> に限定しました。", channel.ID), colorSuccess, false)
}

func (b *Bot) clearChannel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := b.coordinator.ClearTextChannel(ctx, i.GuildID)
	cancel()
	if err != nil {
		b.replyError(s, i, "チャンネル設定を解除できません", err.Error())
		return
	}
	if value, ok := b.rooms.Load(i.GuildID); ok {
		r := value.(*room)
		r.mu.Lock()
		r.textChannelID = ""
		r.mu.Unlock()
	}
	b.replyEmbed(s, i, "📣 読み上げチャンネル設定を解除しました", "自動読み上げを全テキストチャンネルで受け付けます。", colorSuccess, false)
}

func (b *Bot) setSpeaker(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		b.replyError(s, i, "話者IDがありません", "VOICEVOXのスタイルIDを指定してください。")
		return
	}
	speakerID := int(options[0].IntValue())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	speakers, err := b.engine.Speakers(ctx)
	cancel()
	if err != nil {
		b.replyError(s, i, "話者一覧を取得できません", err.Error())
		return
	}
	if !containsSpeaker(speakers, speakerID) {
		b.replyError(s, i, "話者IDを確認してください", fmt.Sprintf("VOICEVOXにスタイルID `%d` がありません。`/speakers`で確認できます。", speakerID))
		return
	}
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	err = b.coordinator.SetSpeaker(ctx, i.GuildID, speakerID)
	cancel()
	if err != nil {
		b.replyError(s, i, "話者を設定できません", err.Error())
		return
	}
	b.replyEmbed(s, i, "🗣️ 標準話者を設定しました", fmt.Sprintf("このサーバーの標準話者をスタイルID `%d` にしました。", speakerID), colorSuccess, false)
}

func (b *Bot) resetSpeaker(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := b.coordinator.ClearSpeaker(ctx, i.GuildID)
	cancel()
	if err != nil {
		b.replyError(s, i, "話者設定を解除できません", err.Error())
		return
	}
	b.replyEmbed(s, i, "🗣️ 標準話者を初期値へ戻しました", fmt.Sprintf("標準話者をスタイルID `%d` に戻しました。", b.defaultSpeaker), colorSuccess, false)
}

func (b *Bot) listSpeakers(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	speakers, err := b.engine.Speakers(ctx)
	cancel()
	if err != nil {
		b.replyError(s, i, "話者一覧を取得できません", err.Error())
		return
	}
	lines := make([]string, 0)
	for _, speaker := range speakers {
		styles := make([]string, 0, len(speaker.Styles))
		for _, style := range speaker.Styles {
			styles = append(styles, fmt.Sprintf("`%d` %s", style.ID, style.Name))
		}
		lines = append(lines, fmt.Sprintf("**%s**\n%s", speaker.Name, strings.Join(styles, "、")))
	}
	if len(lines) == 0 {
		b.replyError(s, i, "話者がありません", "VOICEVOXから話者情報を取得できませんでした。")
		return
	}
	b.replyEmbed(s, i, "🗣️ 利用可能なVOICEVOX話者", strings.Join(lines, "\n"), colorInfo, true)
}

func containsSpeaker(speakers []voicevox.Speaker, speakerID int) bool {
	for _, speaker := range speakers {
		for _, style := range speaker.Styles {
			if style.ID == speakerID {
				return true
			}
		}
	}
	return false
}

func (b *Bot) pause(s *discordgo.Session, i *discordgo.InteractionCreate) {
	room, ok := b.roomForGuild(i.GuildID)
	if !ok {
		b.replyError(s, i, "接続されていません", "先に `/join` を実行してください。")
		return
	}
	room.mu.Lock()
	room.paused = true
	room.mu.Unlock()
	b.replyEmbed(s, i, "⏸️ 読み上げを一時停止しました", "`/resume`で再開できます。", colorInfo, false)
}

func (b *Bot) resume(s *discordgo.Session, i *discordgo.InteractionCreate) {
	room, ok := b.roomForGuild(i.GuildID)
	if !ok {
		b.replyError(s, i, "接続されていません", "先に `/join` を実行してください。")
		return
	}
	room.mu.Lock()
	room.paused = false
	room.mu.Unlock()
	b.replyEmbed(s, i, "▶️ 読み上げを再開しました", "読み上げキューの処理を続行します。", colorSuccess, false)
}

func (b *Bot) clearQueue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	room, ok := b.roomForGuild(i.GuildID)
	if !ok {
		b.replyError(s, i, "接続されていません", "先に `/join` を実行してください。")
		return
	}
	room.mu.Lock()
	count := len(room.queue)
	room.queue = nil
	room.mu.Unlock()
	b.replyEmbed(s, i, "🧹 読み上げキューを消去しました", fmt.Sprintf("待機中の%d件を消去しました。再生中の文章は継続します。", count), colorSuccess, false)
}

func (b *Bot) roomForGuild(guildID string) (*room, bool) {
	value, ok := b.rooms.Load(guildID)
	if !ok {
		return nil, false
	}
	return value.(*room), true
}

func (b *Bot) status(s *discordgo.Session, i *discordgo.InteractionCreate) {
	textChannel := "全テキストチャンネル"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	configured, err := b.coordinator.GetTextChannel(ctx, i.GuildID)
	cancel()
	if err == nil && configured != "" {
		textChannel = fmt.Sprintf("<#%s>", configured)
	}
	speakerID := b.engineDefault(i.GuildID)
	readingState := "未接続"
	queueSize := 0
	if room, ok := b.roomForGuild(i.GuildID); ok {
		room.mu.Lock()
		queueSize = len(room.queue)
		if room.paused {
			readingState = "一時停止中"
		} else {
			readingState = "再生可能"
		}
		room.mu.Unlock()
	}
	fields := []*discordgo.MessageEmbedField{
		{Name: "🤖 BOT", Value: b.name, Inline: true},
		{Name: "🎙️ 接続ルーム", Value: fmt.Sprintf("%d", countRooms(b)), Inline: true},
		{Name: "🗣️ 音声エンジン", Value: "VOICEVOX", Inline: true},
		{Name: "📣 読み上げチャンネル", Value: textChannel, Inline: false},
		{Name: "🗣️ 標準話者", Value: fmt.Sprintf("スタイルID `%d`", speakerID), Inline: true},
		{Name: "⏯️ 読み上げ状態", Value: fmt.Sprintf("%s / 待機%d件", readingState, queueSize), Inline: true},
	}
	if b.guildID == "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "📡 コマンド登録", Value: "グローバル", Inline: true})
	} else {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "📡 コマンド登録", Value: "このサーバー", Inline: true})
	}

	b.replyEmbedFields(s, i, "📊 mogkoe ステータス", "正常に稼働しています。", colorSuccess, fields, false)
}

func (b *Bot) enqueue(room *room, text string, speaker int) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	audio, err := b.engine.Synthesize(ctx, text, speaker)
	if err != nil {
		b.log.Error("音声合成に失敗しました", "error", err)
		return
	}

	room.mu.Lock()
	room.queue = append(room.queue, audio)
	if room.playing {
		room.mu.Unlock()
		return
	}
	room.playing = true
	room.mu.Unlock()

	for {
		room.mu.Lock()
		if room.paused {
			room.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if len(room.queue) == 0 {
			room.playing = false
			room.mu.Unlock()
			return
		}
		audio = room.queue[0]
		room.queue = room.queue[1:]
		room.mu.Unlock()

		for _, pcm := range pcmFrames(audio) {
			for {
				room.mu.Lock()
				paused := room.paused
				room.mu.Unlock()
				if !paused {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			frame := make([]byte, 4000)
			pcm16 := pcmBytesToInt16(pcm)
			n, err := room.encoder.Encode(pcm16, frame)
			if err != nil {
				b.log.Error("Opusエンコードに失敗しました", "error", err)
				continue
			}
			room.voice.OpusSend <- frame[:n]
		}
	}
}

func (r *room) queueLength() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.queue)
}

func (r *room) textChannel() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.textChannelID
}

func pcmBytesToInt16(data []byte) []int16 {
	pcm := make([]int16, len(data)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
	}
	return pcm
}

func pcmFrames(data []byte) [][]byte {
	const frameSize = 3840
	frames := make([][]byte, 0, (len(data)+frameSize-1)/frameSize)
	for len(data) > 0 {
		size := frameSize
		if len(data) < size {
			size = len(data)
		}
		frame := make([]byte, frameSize)
		copy(frame, data[:size])
		frames = append(frames, frame)
		data = data[size:]
	}
	return frames
}

func autoSpeaker(text string) int {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "!") || strings.Contains(text, "？") || strings.Contains(text, "?"):
		return 8
	case strings.Contains(text, "おはよう") || strings.Contains(text, "こんばんは"):
		return 1
	case strings.Contains(text, "ありがとう") || strings.Contains(text, "よろしく"):
		return 3
	default:
		return 3
	}
}

func (b *Bot) engineDefault(guildID string) int {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	speaker, err := b.coordinator.GetSpeaker(ctx, guildID, b.defaultSpeaker)
	if err != nil {
		return b.defaultSpeaker
	}
	return speaker
}

func (b *Bot) replyError(s *discordgo.Session, i *discordgo.InteractionCreate, title, description string) {
	b.replyEmbed(s, i, "❌ "+title, description, colorError, true)
}

func (b *Bot) replyEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, title, description string, color int, ephemeral bool) {
	b.replyEmbedFields(s, i, title, description, color, nil, ephemeral)
}

func (b *Bot) replyEmbedFields(s *discordgo.Session, i *discordgo.InteractionCreate, title, description string, color int, fields []*discordgo.MessageEmbedField, ephemeral bool) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: flags,
			Embeds: []*discordgo.MessageEmbed{{
				Title:       title,
				Description: description,
				Color:       color,
				Fields:      fields,
				Footer:      &discordgo.MessageEmbedFooter{Text: "mogkoe（もじこえ） • VOICEVOX"},
			}},
		},
	})
}

func countRooms(b *Bot) int {
	count := 0
	b.rooms.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
