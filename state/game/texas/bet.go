package texas

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/ratel-online/core/log"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
	"github.com/spf13/cast"
)

func bet(player *database.Player, game *database.Texas) error {
	texasPlayer := game.Player(player.ID)

	if game.RoundEnd(player.ID) {
		return nextRound(game)
	}
	if texasPlayer.Folded || texasPlayer.AllIn {
		return nextPlayer(player, game, stateBet)
	}
	player.StartTransaction()
	defer player.StopTransaction()

	database.Broadcast(player.RoomID, fmt.Sprintf("轮到 %s 下注。\n", player.Name), player.ID)

	timeout := consts.BetTimeout
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[bet] Player %d (Room %d) loop count: %d, timeout: %v\n", player.ID, player.RoomID, loopCount, timeout)
		}
		before := time.Now().Unix()

		buf := bytes.Buffer{}
		buf.WriteString(fmt.Sprintf("你的手牌：%s\n", texasPlayer.Hand.TexasString()))
		for _, p := range game.Players {
			status := "下注中"
			if p.Folded {
				status = "已弃牌"
			}
			if p.AllIn {
				status = "全下"
			}
			name := p.Name
			if p.ID == player.ID {
				name = "* 你"
			}
			buf.WriteString(fmt.Sprintf("%s 剩余积分：%d，累计下注：%d，状态：%s\n", name, p.Amount(), p.Bets, status))
		}
		buf.WriteString("请选择操作（call/raise/fold/check/allin）：\n")
		_ = player.WriteString(buf.String())
		ans, err := player.AskForStringWithoutTransaction(timeout)
		if err != nil {
			ans = "fold"
		}
		timeout -= time.Second * time.Duration(time.Now().Unix()-before)
		minCall := game.MaxBetAmount - texasPlayer.Bets

		instructions := strings.Split(ans, " ")
		switch instructions[0] {
		case "call":
			if minCall == 0 {
				_ = player.WriteString("当前无需跟注，你可以输入 check 过牌。\n")
				continue
			}
			if texasPlayer.Amount() < minCall {
				_ = player.WriteString("你的积分不足，无法跟注。\n")
				continue
			}
			game.Bet(texasPlayer, minCall)
			database.Broadcast(player.RoomID, fmt.Sprintf("%s 跟注 %d。\n", player.Name, minCall))
		case "raise":
			if len(instructions) <= 1 || instructions[1] == "" {
				_ = player.WriteString("请输入要加注的金额，例如：raise 100。\n")
				continue
			}
			betAmount, err := cast.ToUintE(instructions[1])
			if err != nil {
				_ = player.WriteString("金额无效。\n")
				continue
			}
			if betAmount < minCall {
				_ = player.WriteString(fmt.Sprintf("加注金额不能低于当前最低跟注金额 %d。\n", minCall))
				continue
			}
			if texasPlayer.Amount() < betAmount {
				_ = player.WriteString("你的积分不足，无法加注。\n")
				continue
			}
			game.Bet(texasPlayer, betAmount)
			database.Broadcast(player.RoomID, fmt.Sprintf("%s 加注 %d。\n", player.Name, betAmount))
		case "fold":
			texasPlayer.Folded = true
			game.Folded++
			database.Broadcast(player.RoomID, fmt.Sprintf("%s 弃牌。\n", player.Name))
			if game.Folded == len(game.Players)-1 {
				return settlementRound(game)
			}
		case "check":
			if texasPlayer.Bets < game.MaxBetAmount {
				_ = player.WriteString("其他玩家的下注更高，你不能过牌。\n")
				continue
			}
			game.Bet(texasPlayer, 0)
			database.Broadcast(player.RoomID, fmt.Sprintf("%s 过牌。\n", player.Name))
		case "allin":
			betAmount := texasPlayer.Amount()
			game.Bet(texasPlayer, betAmount)
			database.Broadcast(player.RoomID, fmt.Sprintf("%s 全下，共下注 %d。\n", player.Name, betAmount))
		default:
			database.BroadcastChat(player, fmt.Sprintf("%s [%s] 说：%s\n", player.Name, database.RoleName(player.Role), ans))
			continue
		}
		break
	}
	return nextPlayer(player, game, stateBet)
}
