module github.com/kawaewanko/mogkoe

go 1.22

require (
	github.com/bwmarrin/discordgo v0.29.0
	github.com/redis/go-redis/v9 v9.7.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f
)

require (
	github.com/gorilla/websocket v1.4.2
	github.com/hraban/opus v0.0.0-20260708213942-bde8e4304501
)

replace github.com/bwmarrin/discordgo => ./third_party/discordgo
