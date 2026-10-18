package coordination

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	leaderTTL  = 20 * time.Second
	messageTTL = 30 * time.Minute
	healthTTL  = 45 * time.Second
)

type Coordinator struct {
	client *redis.Client
	log    *slog.Logger
	botID  string
}

func New(redisURL, botID string, log *slog.Logger) (*Coordinator, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("REDIS_URLの解析に失敗しました: %w", err)
	}
	return &Coordinator{client: redis.NewClient(options), log: log, botID: botID}, nil
}

func (c *Coordinator) Close() error { return c.client.Close() }

func (c *Coordinator) PublishHealth(ctx context.Context, healthy bool, latencyMS time.Duration) error {
	state := "unhealthy"
	if healthy {
		state = "healthy"
	}
	return c.client.Set(ctx, "mogkoe:health:"+c.botID, state, healthTTL).Err()
}

func (c *Coordinator) SetTextChannel(ctx context.Context, guildID, channelID string) error {
	return c.client.Set(ctx, "mogkoe:text-channel:"+guildID, channelID, 0).Err()
}

func (c *Coordinator) GetTextChannel(ctx context.Context, guildID string) (string, error) {
	value, err := c.client.Get(ctx, "mogkoe:text-channel:"+guildID).Result()
	if err == redis.Nil {
		return "", nil
	}
	return value, err
}

func (c *Coordinator) ClearTextChannel(ctx context.Context, guildID string) error {
	return c.client.Del(ctx, "mogkoe:text-channel:"+guildID).Err()
}

func (c *Coordinator) SetSpeaker(ctx context.Context, guildID string, speakerID int) error {
	return c.client.Set(ctx, "mogkoe:speaker:"+guildID, speakerID, 0).Err()
}

func (c *Coordinator) GetSpeaker(ctx context.Context, guildID string, fallback int) (int, error) {
	value, err := c.client.Get(ctx, "mogkoe:speaker:"+guildID).Int()
	if err == redis.Nil {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	return value, nil
}

func (c *Coordinator) ClearSpeaker(ctx context.Context, guildID string) error {
	return c.client.Del(ctx, "mogkoe:speaker:"+guildID).Err()
}

func (c *Coordinator) ClaimMessage(ctx context.Context, guildID, messageID string) (bool, error) {
	leaderKey := "mogkoe:leader:" + guildID
	messageKey := "mogkoe:message:" + guildID + ":" + messageID
	healthKey := "mogkoe:health:" + c.botID
	ownHealth, err := c.client.Get(ctx, healthKey).Result()
	if err != nil && err != redis.Nil {
		return false, err
	}
	if ownHealth == "unhealthy" {
		return false, nil
	}

	claimed, err := c.client.SetNX(ctx, leaderKey, c.botID, leaderTTL).Result()
	if err != nil {
		c.log.Error("リーダー選出に失敗しました", "guild_id", guildID, "error", err)
		return false, err
	}
	if !claimed {
		leader, err := c.client.Get(ctx, leaderKey).Result()
		if err != nil {
			return false, err
		}
		if leader != c.botID {
			leaderHealth, healthErr := c.client.Get(ctx, "mogkoe:health:"+leader).Result()
			if healthErr != nil && healthErr != redis.Nil {
				return false, healthErr
			}
			if leaderHealth != "unhealthy" {
				return false, nil
			}
			if err := c.client.Del(ctx, leaderKey).Err(); err != nil {
				return false, err
			}
			claimed, err = c.client.SetNX(ctx, leaderKey, c.botID, leaderTTL).Result()
			if err != nil || !claimed {
				return false, err
			}
		} else {
			_ = c.client.Expire(ctx, leaderKey, leaderTTL).Err()
		}
	}

	first, err := c.client.SetNX(ctx, messageKey, c.botID, messageTTL).Result()
	if err != nil {
		c.log.Error("メッセージ処理権の取得に失敗しました", "guild_id", guildID, "message_id", messageID, "error", err)
		return false, err
	}
	return first, nil
}
