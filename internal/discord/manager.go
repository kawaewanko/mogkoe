package discord

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/hraban/opus"
	"github.com/kawaewanko/mogkoe/internal/config"
	"github.com/kawaewanko/mogkoe/internal/coordination"
	"github.com/kawaewanko/mogkoe/internal/voicevox"
)

type Manager struct {
	cfg      config.Config
	engine   *voicevox.Engine
	log      *slog.Logger
	sessions []*Bot
	wg       sync.WaitGroup
}

type Bot struct {
	name                  string
	number                int
	session               *discordgo.Session
	engine                *voicevox.Engine
	guildID               string
	voicevoxURL           string
	defaultSpeaker        int
	log                   *slog.Logger
	rooms                 sync.Map
	coordinator           *coordination.Coordinator
	presenceStop          chan struct{}
	pollInterval          time.Duration
	latencyLimit          time.Duration
	failureLimit          int
	recoveryLimit         int
	healthMu              sync.Mutex
	healthy               bool
	consecutiveFailures   int
	consecutiveRecoveries int
}

type room struct {
	voice         *discordgo.VoiceConnection
	encoder       *opus.Encoder
	textChannelID string
	mu            sync.Mutex
	playing       bool
	paused        bool
	queue         [][]byte
}

func NewManager(cfg config.Config, engine *voicevox.Engine, log *slog.Logger) *Manager {
	return &Manager{cfg: cfg, engine: engine, log: log}
}

func (m *Manager) Run(ctx context.Context) error {
	for _, botConfig := range m.cfg.Bots {
		if strings.TrimSpace(botConfig.Token) == "" {
			return fmt.Errorf("Discord BOTトークンが設定されていません")
		}

		bot, err := newBot(botConfig, m.cfg, m.engine, m.log)
		if err != nil {
			return err
		}

		m.sessions = append(m.sessions, bot)
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := bot.start(); err != nil {
				m.log.Error("BOTが停止しました", "bot", bot.name, "error", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-ctx.Done():
	case <-stop:
	}

	for _, bot := range m.sessions {
		_ = bot.session.Close()
		_ = bot.coordinator.Close()
		close(bot.presenceStop)
	}
	m.wg.Wait()
	return nil
}

func newBot(c config.BotConfig, cfg config.Config, engine *voicevox.Engine, log *slog.Logger) (*Bot, error) {
	session, err := discordgo.New("Bot " + c.Token)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		name:           c.Name,
		number:         c.Number,
		session:        session,
		engine:         engine,
		guildID:        cfg.GuildID,
		voicevoxURL:    cfg.VoicevoxURL,
		defaultSpeaker: cfg.DefaultSpeaker,
		log:            log,
		presenceStop:   make(chan struct{}),
		pollInterval:   cfg.LatencyPollInterval,
		latencyLimit:   cfg.LatencyThreshold,
		failureLimit:   cfg.LatencyFailureLimit,
		recoveryLimit:  cfg.LatencyRecoveryLimit,
		healthy:        true,
	}
	coordinator, err := coordination.New(cfg.RedisURL, c.Name, log)
	if err != nil {
		return nil, err
	}
	bot.coordinator = coordinator
	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteraction)
	session.AddHandler(bot.onMessage)
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildVoiceStates |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent
	return bot, nil
}

func (b *Bot) start() error {
	ready := make(chan struct{})
	b.session.AddHandlerOnce(func(*discordgo.Session, *discordgo.Ready) {
		close(ready)
	})
	if err := b.session.Open(); err != nil {
		return err
	}
	<-ready
	go b.presenceLoop()
	return nil
}

func (b *Bot) presenceLoop() {
	b.updatePresence()
	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.updatePresence()
		case <-b.presenceStop:
			return
		}
	}
}

func (b *Bot) updatePresence() {
	delayDuration := b.session.HeartbeatLatency()
	delay := delayDuration / time.Millisecond
	healthy := b.updateHealth(delayDuration)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := b.coordinator.PublishHealth(ctx, healthy, delay); err != nil {
		b.log.Error("BOTのヘルス状態共有に失敗しました", "bot", b.name, "error", err)
	}
	cancel()
	text := fmt.Sprintf("/help｜%dms｜%d号機", delay, b.number)
	if err := b.session.UpdateWatchStatus(0, text); err != nil {
		b.log.Error("視聴中ステータスの更新に失敗しました", "bot", b.name, "error", err)
	}
}

func (b *Bot) updateHealth(delay time.Duration) bool {
	b.healthMu.Lock()
	defer b.healthMu.Unlock()
	if !b.session.DataReady || delay > b.latencyLimit {
		b.consecutiveFailures++
		b.consecutiveRecoveries = 0
		if b.consecutiveFailures >= b.failureLimit {
			b.healthy = false
		}
		return b.healthy
	}
	b.consecutiveFailures = 0
	b.consecutiveRecoveries++
	if b.consecutiveRecoveries >= b.recoveryLimit {
		b.healthy = true
	}
	return b.healthy
}

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	if _, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, b.guildID, commands()); err != nil {
		b.log.Error("スラッシュコマンドの登録に失敗しました", "bot", b.name, "guild", b.guildID, "error", err)
		return
	}
	b.log.Info("BOTの準備が完了しました", "bot", b.name, "user", s.State.User.Username)
}

func commands() []*discordgo.ApplicationCommand {
	textOption := func(description string) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "text",
			Description: description,
			Required:    true,
			MaxLength:   200,
		}
	}
	return []*discordgo.ApplicationCommand{
		{Name: "help", Description: "利用できるコマンドを表示"},
		{Name: "join", Description: "あなたがいるボイスチャンネルへ参加"},
		{Name: "leave", Description: "ボイスチャンネルから退出"},
		{Name: "speak", Description: "標準話者で文章を読み上げ", Options: []*discordgo.ApplicationCommandOption{textOption("読み上げる文章（最大200文字）")}},
		{Name: "voice", Description: "文章に合う話者を自動選定して読み上げ", Options: []*discordgo.ApplicationCommandOption{textOption("読み上げる文章（最大200文字）")}},
		{Name: "channel", Description: "自動読み上げチャンネルを指定", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "読み上げるテキストチャンネル", Required: true, ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews}}}},
		{Name: "channel-off", Description: "自動読み上げチャンネルの指定を解除"},
		{Name: "speaker", Description: "サーバーの標準話者を設定", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionInteger, Name: "speaker_id", Description: "VOICEVOXのスタイルID", Required: true, MinValue: float64Ptr(0), MaxValue: 10000}}},
		{Name: "speaker-reset", Description: "標準話者を初期値へ戻す"},
		{Name: "speakers", Description: "利用可能なVOICEVOX話者を表示"},
		{Name: "pause", Description: "読み上げを一時停止"},
		{Name: "resume", Description: "読み上げを再開"},
		{Name: "clear", Description: "読み上げキューを消去"},
		{Name: "status", Description: "mogkoeの状態を表示"},
	}
}

func float64Ptr(value float64) *float64 { return &value }
