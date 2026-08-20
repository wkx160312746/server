package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/ratel-online/core/log"
	"github.com/ratel-online/core/util/async"
	"github.com/ratel-online/server/bot"
	"github.com/ratel-online/server/network"
)

var (
	Wsport   int
	Tcpport  int
	BotAddr  string
	BotToken string
	BotGroup int64
)

func main() {
	flag.IntVar(&Wsport, "w", envInt("PORT", 9998), "WebsocketServer Port")
	flag.IntVar(&Tcpport, "t", envInt("TCP_PORT", 9999), "TcpServer Port")
	flag.StringVar(&BotAddr, "bot", os.Getenv("BOT_ADDR"), "Bot connection address")
	flag.StringVar(&BotToken, "bot-token", os.Getenv("BOT_TOKEN"), "Bot token")
	flag.Int64Var(&BotGroup, "bot-group", envInt64("BOT_GROUP", 0), "Bot group ID")

	flag.Parse()
	// 连接机器人
	if BotAddr != "" && BotToken != "" && BotGroup != 0 {
		err := bot.Connect(BotAddr, BotToken, BotGroup)
		if err != nil {
			log.Panic(fmt.Sprintf("连接Bot失败: %v", err))
		}
		// 发送测试消息到 BotGroup 群
		err = bot.SendGroupMessage(BotGroup, "Server started!")
		if err != nil {
			log.Errorf("发送群消息失败: %v", err)
		} else {
			log.Infof("已发送群消息到 %d", BotGroup)
		}
		defer bot.Close()
	}

	async.Async(func() {
		wsServer := network.NewWebsocketServer(":" + strconv.Itoa(Wsport))
		log.Panic(wsServer.Serve())
	})

	server := network.NewTcpServer(":" + strconv.Itoa(Tcpport))
	log.Panic(server.Serve())
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Errorf("Invalid %s value %q, using %d: %v", name, value, fallback, err)
		return fallback
	}
	return parsed
}

func envInt64(name string, fallback int64) int64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		log.Errorf("Invalid %s value %q, using %d: %v", name, value, fallback, err)
		return fallback
	}
	return parsed
}
