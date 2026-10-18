package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type BotConfig struct {
	Name   string
	Token  string
	Number int
}
type Config struct {
	Bots                  []BotConfig
	GuildID, RedisURL     string
	VoicevoxURL, CacheDir string
	DefaultSpeaker        int
	AudioCacheTTL         time.Duration
	LatencyPollInterval   time.Duration
	LatencyThreshold      time.Duration
	LatencyFailureLimit   int
	LatencyRecoveryLimit  int
}

func Load() Config {
	bots := make([]BotConfig, 0, 4)
	for i := 1; i <= 10; i++ {
		token := strings.TrimSpace(os.Getenv("BOT_" + strconv.Itoa(i) + "_TOKEN"))
		if token != "" && token != "replace-me" {
			bots = append(bots, BotConfig{Name: "mogkoe-" + strconv.Itoa(i), Token: token, Number: i})
		}
	}
	if len(bots) == 0 {
		bots = append(bots, BotConfig{Name: "mogkoe-1", Token: os.Getenv("DISCORD_TOKEN"), Number: 1})
	}
	speaker, _ := strconv.Atoi(env("VOICE_DEFAULT_SPEAKER", "3"))
	ttl, _ := strconv.Atoi(env("AUDIO_CACHE_TTL", "3600"))
	pollSeconds, _ := strconv.Atoi(env("LATENCY_POLL_INTERVAL_SECONDS", "15"))
	thresholdMS, _ := strconv.Atoi(env("LATENCY_ALERT_THRESHOLD_MS", "1500"))
	failureLimit, _ := strconv.Atoi(env("LATENCY_FAILURE_LIMIT", "3"))
	recoveryLimit, _ := strconv.Atoi(env("LATENCY_RECOVERY_LIMIT", "2"))
	return Config{
		Bots:                 bots,
		GuildID:              os.Getenv("DISCORD_GUILD_ID"),
		RedisURL:             env("REDIS_URL", "redis://redis:6379/0"),
		VoicevoxURL:          env("VOICEVOX_URL", "http://localhost:50021"),
		CacheDir:             env("CACHE_DIR", "cache"),
		DefaultSpeaker:       speaker,
		AudioCacheTTL:        time.Duration(ttl) * time.Second,
		LatencyPollInterval:  time.Duration(maxInt(pollSeconds, 5)) * time.Second,
		LatencyThreshold:     time.Duration(maxInt(thresholdMS, 100)) * time.Millisecond,
		LatencyFailureLimit:  maxInt(failureLimit, 1),
		LatencyRecoveryLimit: maxInt(recoveryLimit, 1),
	}
}

func maxInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
