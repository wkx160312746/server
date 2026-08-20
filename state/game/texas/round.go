package texas

import (
	"bytes"
	"fmt"

	"github.com/ratel-online/core/model"
	"github.com/ratel-online/core/util/poker"
	"github.com/ratel-online/server/bot"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
)

func nextRound(game *database.Texas) error {
	switch game.Round {
	case "start":
		return preFlopRound(game)
	case "per-flop":
		return flopRound(game)
	case "flop":
		return turnRound(game)
	case "turn":
		return riverRound(game)
	case "river":
		return settlementRound(game)
	default:
		return consts.ErrorsUnknownTexasRound
	}
}

func preFlopRound(game *database.Texas) error {
	game.Round = "per-flop"
	for id := range database.RoomPlayers(game.Room.ID) {
		player := database.GetPlayer(id)
		if player.Amount < 100 {
			player.Amount += 2000
			database.Broadcast(game.Room.ID, fmt.Sprintf("%s 的积分过低，系统赠送了 2000 积分。\n", player.Name))
			bot.SendGroupMessage(bot.GroupID, fmt.Sprintf("%s 的积分过低，系统赠送了 2000 积分。", player.Name))
		}
	}

	game.Pot += 30
	game.BBPlayer().Bet(20)
	game.SBPlayer().Bet(10)

	for id := range database.RoomPlayers(game.Room.ID) {
		player := database.GetPlayer(id)
		texasPlayer := game.Player(id)

		buf := bytes.Buffer{}
		buf.WriteString("游戏开始！\n")
		if game.SBPlayer().ID != player.ID {
			buf.WriteString(fmt.Sprintf("你的手牌：%s\n", texasPlayer.Hand.TexasString()))
		}
		if game.BBPlayer().ID == player.ID {
			buf.WriteString("你是大盲，已自动下注 20。\n")
		} else {
			buf.WriteString(fmt.Sprintf("大盲：%s，下注 20。\n", game.Players[game.BB].Name))
		}
		if game.SBPlayer().ID == player.ID {
			buf.WriteString("你是小盲，已自动下注 10。\n")
		} else {
			buf.WriteString(fmt.Sprintf("小盲：%s，下注 10。\n", game.Players[game.SB].Name))
			buf.WriteString(fmt.Sprintf("翻牌前回合，请等待小盲 %s 下注。\n", game.Players[game.SB].Name))
		}
		_ = player.WriteString(buf.String())
	}
	game.SBPlayer().State <- stateBet
	return nil
}

func flopRound(game *database.Texas) error {
	game.Round = "flop"
	game.MaxBetPlayer = nil
	game.Board = append(game.Board, game.Pool[1:4]...)
	game.Pool = game.Pool[4:]
	database.Broadcast(game.Room.ID, fmt.Sprintf("翻牌回合，公共牌：%s\n", game.Board.TexasString()))
	game.SBPlayer().State <- stateBet
	return nil
}

func turnRound(game *database.Texas) error {
	game.Round = "turn"
	game.MaxBetPlayer = nil
	game.Board = append(game.Board, game.Pool[1:2]...)
	game.Pool = game.Pool[2:]
	database.Broadcast(game.Room.ID, fmt.Sprintf("转牌回合，公共牌：%s\n", game.Board.TexasString()))
	game.SBPlayer().State <- stateBet
	return nil
}

func riverRound(game *database.Texas) error {
	game.Round = "river"
	game.MaxBetPlayer = nil
	game.Board = append(game.Board, game.Pool[1:2]...)
	game.Pool = game.Pool[2:]
	database.Broadcast(game.Room.ID, fmt.Sprintf("河牌回合，公共牌：%s\n", game.Board.TexasString()))
	game.SBPlayer().State <- stateBet
	return nil
}

func settlementRound(game *database.Texas) error {
	buf := bytes.Buffer{}
	buf.WriteString("结算回合\n")
	buf.WriteString(fmt.Sprintf("公共牌：%s\n", game.Board.TexasString()))

	if game.Folded == len(game.Players)-1 {
		var winner *database.TexasPlayer
		for _, player := range game.Players {
			if !player.Folded {
				winner = player
				break
			}
		}
		if winner != nil {
			winner.Add(game.Pot)
			buf.WriteString(fmt.Sprintf("获胜者：%s，赢得全部底池 %d。\n", winner.Name, game.Pot))
		} else {
			buf.WriteString("所有玩家均已弃牌。\n")
		}
	} else {
		buf.WriteString("玩家手牌：\n")
		var maxFaces *model.TexasFaces
		var maxPlayers []int64
		for _, player := range game.Players {
			if player.Folded {
				continue
			}
			faces, err := poker.ParseTexasFaces(player.Hand, game.Board)
			if err != nil {
				return err
			}
			buf.WriteString(fmt.Sprintf("%s：%s，牌型：%s，点数：%d\n", player.Name, player.Hand.TexasString(), texasFaceName(faces.Type), faces.Score))
			if maxFaces == nil ||
				maxFaces.Type < faces.Type ||
				(maxFaces.Type == faces.Type && maxFaces.Score < faces.Score) {
				maxFaces = faces
				maxPlayers = []int64{player.ID}
				continue
			}
			if maxFaces.Type == faces.Type && maxFaces.Score == faces.Score {
				maxPlayers = append(maxPlayers, player.ID)
			}
		}
		winners := make([]*database.TexasPlayer, 0)
		for _, id := range maxPlayers {
			winners = append(winners, game.Player(id))
		}
		if len(winners) == 1 {
			buf.WriteString(fmt.Sprintf("获胜者：%s，赢得全部底池 %d。\n", winners[0].Name, game.Pot))
		} else {
			buf.WriteString("获胜者：")
			for i, winner := range winners {
				if i != 0 {
					buf.WriteString("、")
				}
				buf.WriteString(winner.Name)
			}
			buf.WriteString(fmt.Sprintf("，平分底池 %d。\n", game.Pot))
		}
		for _, winner := range winners {
			winner.Add(game.Pot / uint(len(winners)))
		}
	}
	buf.WriteString(fmt.Sprintf("请房主 %s 开始新一局游戏。\n", database.GetPlayer(game.Room.Creator).Name))
	database.Broadcast(game.Room.ID, buf.String())

	room := game.Room
	room.State = consts.RoomStateWaiting
	for _, player := range game.Players {
		player.State <- stateWaiting
	}
	return nil
}

func texasFaceName(faceType model.TexasFacesType) string {
	switch faceType {
	case model.TexasFacesTypeHigh:
		return "高牌"
	case model.TexasFacesTypeOnePair:
		return "一对"
	case model.TexasFacesTypeTwoPairs:
		return "两对"
	case model.TexasFacesTypeThreeOfAKind:
		return "三条"
	case model.TexasFacesTypeStraight:
		return "顺子"
	case model.TexasFacesTypeFlush:
		return "同花"
	case model.TexasFacesTypeFullHouse:
		return "葫芦"
	case model.TexasFacesTypeFourOfAKind:
		return "四条"
	case model.TexasFacesTypeStraightFlush:
		return "同花顺"
	case model.TexasFacesTypeRoyalFlush:
		return "皇家同花顺"
	default:
		return "未知牌型"
	}
}
