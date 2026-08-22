package state

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/ratel-online/server/state/game/texas"
	"github.com/spf13/cast"

	"github.com/ratel-online/core/log"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
	"github.com/ratel-online/server/rule"
	"github.com/ratel-online/server/state/game"
)

type waiting struct{}

func (s *waiting) Next(player *database.Player) (consts.StateID, error) {
	room := database.GetRoom(player.RoomID)
	if room == nil {
		return 0, consts.ErrorsExist
	}
	s.Backfill(room)

	access, err := s.waitingForStart(player, room)
	if err != nil {
		return 0, err
	}
	if access {
		switch room.Type {
		default:
			return consts.StateGame, nil
		case consts.GameTypeRunFast:
			return consts.StateRunFastGame, nil
		case consts.GameTypeUno:
			return consts.StateUnoGame, nil
		case consts.GameTypeMahjong:
			return consts.StateMahjongGame, nil
		case consts.GameTypeTexas:
			return consts.StateTexasGame, nil
		case consts.GameTypeLiar:
			return consts.StateLiarGame, nil
		case consts.GameTypeUndercover:
			return consts.StateUndercoverGame, nil
		}
	}
	return s.Exit(player), nil
}

func (s *waiting) Exit(player *database.Player) consts.StateID {
	room := database.GetRoom(player.RoomID)
	if room != nil {
		isOwner := room.Creator == player.ID
		database.LeaveRoom(room.ID, player.ID)
		database.Broadcast(room.ID, fmt.Sprintf("%s 离开了房间，当前共有 %d 名玩家。\n", player.Name, room.Players))
		if isOwner {
			newOwner := database.GetPlayer(room.Creator)
			database.Broadcast(room.ID, fmt.Sprintf("%s 成为新房主。\n", newOwner.Name))
		}
		s.Backfill(room)
	}
	return consts.StateHome
}

func (*waiting) Backfill(room *database.Room) {
	if room.State == consts.RoomStateRunning {
		return
	}
	newPlayer := database.Backfill(room.ID)
	if newPlayer != nil {
		database.Broadcast(room.ID, fmt.Sprintf("%s 从观战席补位成为玩家，当前共有 %d 名玩家。\n", newPlayer.Name, room.Players))
	}
}

func (*waiting) Kicking(player *database.Player) {
	room := database.GetRoom(player.RoomID)
	if room != nil {
		database.Broadcast(room.ID, fmt.Sprintf("%s 已被房主踢出。\n", player.Name))
		database.Kicking(room.ID, player.ID)
		database.Broadcast(room.ID, fmt.Sprintf("房间当前共有 %d 名玩家。\n", room.Players))
	}
}

func (s *waiting) waitingForStart(player *database.Player, room *database.Room) (bool, error) {
	access := false
	//对局类别
	player.StartTransaction()
	defer player.StopTransaction()
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[waitingForStart] Player %d (Room %d) loop count: %d, room.State: %d, access: %v\n", player.ID, player.RoomID, loopCount, room.State, access)
		}
		signal, err := player.AskForStringWithoutTransaction(time.Second)
		if err != nil && err != consts.ErrorsTimeout {
			return access, err
		}

		if !database.IsValidPlayer(room.ID, player.ID) {
			return false, consts.ErrorsPlayerNotInRoom
		}

		if room.State == consts.RoomStateRunning && player.Role == database.RolePlayer {
			access = true
			break
		}
		signal = strings.TrimSpace(strings.ToLower(signal))
		if signal == "" {
			continue
		}

		segments := strings.Split(signal, " ")
		if len(segments) == 1 {
			if segments[0] == "ls" || segments[0] == "v" {
				viewRoomPlayers(room, player)
				continue
			} else if segments[0] == "start" || signal == "s" {
				if room.Creator == player.ID {
					if room.Players <= 1 {
						_ = player.WriteError(consts.ErrorsGamePlayersInsufficient)
						continue
					}
					if room.Type == consts.GameTypeRunFast && room.Players != 3 {
						_ = player.WriteError(consts.ErrorsGamePlayersInvalid)
						continue
					}
					if room.Type == consts.GameTypeUndercover && room.Players < 3 {
						_ = player.WriteString("谁是卧底游戏至少需要3名玩家！\n")
						continue
					}
					err = startGame(player, room)
					if err != nil {
						return access, err
					}
					access = true
					break
				}
			}
		} else if len(segments) == 2 {
			if segments[0] == "kicking" || segments[0] == "kill" || segments[0] == "k" {
				if room.Creator == player.ID {
					kickedId := cast.ToInt64(segments[1])
					if kickedId == player.ID {
						_ = player.WriteError(consts.ErrorsCannotKickYourself)
						continue
					}

					kickedPlayer := database.GetPlayer(kickedId)
					if kickedPlayer == nil || kickedPlayer.RoomID != room.ID {
						_ = player.WriteError(consts.ErrorsPlayerNotInRoom)
						continue
					}

					s.Kicking(kickedPlayer)
					continue
				}
			}
		} else if len(segments) == 3 && room.Creator == player.ID {
			database.SetRoomProps(room, segments[1], segments[2])
			continue
		}

		if room.EnableChat {
			if room.State == consts.RoomStateRunning && room.Type != consts.GameTypeTexas {
				_ = player.WriteString(fmt.Sprintf("%s\n", consts.ErrorsChatUnopenedDuringGame.Error()))
			} else {
				database.BroadcastChat(player, fmt.Sprintf("%s [%s] 说：%s\n", player.Name, database.RoleName(player.Role), signal))
			}
		} else {
			_ = player.WriteString(fmt.Sprintf("%s\n", consts.ErrorsChatUnopened.Error()))
		}
	}
	return access, nil
}

func startGame(player *database.Player, room *database.Room) (err error) {
	room.Lock()
	defer room.Unlock()
	switch room.Type {
	default:
		room.Game, err = game.InitGame(room)
	case consts.GameTypeUno:
		room.Game, err = game.InitUnoGame(room)
	case consts.GameTypeRunFast:
		room.Game, err = game.InitRunFastGame(room, rule.RunFastRules)
	case consts.GameTypeMahjong:
		room.Game, err = game.InitMahjongGame(room)
	case consts.GameTypeTexas:
		room.Game, err = texas.Init(room)
	case consts.GameTypeLiar:
		room.Game, err = game.InitLiarGame(room)
	case consts.GameTypeUndercover:
		room.Game, err = game.InitUndercoverGame(room)
	}
	if err != nil {
		_ = player.WriteError(err)
		return err
	}
	room.State = consts.RoomStateRunning
	return nil
}

func viewRoomPlayers(room *database.Room, currPlayer *database.Player) {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("房间 ID：%d\n", room.ID))
	buf.WriteString("玩家：\n")
	for playerId := range database.RoomPlayers(room.ID) {
		player := database.GetPlayer(playerId)
		if room.EnableShowIP {
			buf.WriteString(fmt.Sprintf("%s [%s]，积分：%d，ID：%d，IP：%s\n", player.Name, database.RoleName(player.Role), player.Amount, player.ID, maskIP(player.IP)))
		} else {
			buf.WriteString(fmt.Sprintf("%s [%s]，积分：%d，ID：%d\n", player.Name, database.RoleName(player.Role), player.Amount, player.ID))
		}
	}

	buf.WriteString("\n观战者：\n")
	for spectatorId := range database.RoomSpectators(room.ID) {
		spectator := database.GetPlayer(spectatorId)
		if room.EnableShowIP {
			buf.WriteString(fmt.Sprintf("%s [观战者]，积分：%d，ID：%d，IP：%s\n", spectator.Name, spectator.Amount, spectator.ID, maskIP(spectator.IP)))
		} else {
			buf.WriteString(fmt.Sprintf("%s [观战者]，积分：%d，ID：%d\n", spectator.Name, spectator.Amount, spectator.ID))
		}
	}

	buf.WriteString("\n房间设置（括号内为设置命令）：\n")
	switch room.Type {
	case consts.GameTypeUno, consts.GameTypeMahjong:
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "显示 IP (ip)：", sprintPropsState(room.EnableShowIP)))
	case consts.GameTypeTexas:
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "人数上限 (pn)：", room.MaxPlayers))
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "显示 IP (ip)：", sprintPropsState(room.EnableShowIP)))
	case consts.GameTypeLiar:
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "王可作指示牌 (jt)：", sprintPropsState(room.EnableJokerAsTarget)))
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "显示 IP (ip)：", sprintPropsState(room.EnableShowIP)))
	case consts.GameTypeUndercover:
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "人数上限 (pn)：", room.MaxPlayers))
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "卧底数量 (ucn)：", room.UndercoverNum))
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "空白词模式 (bwm)：", sprintPropsState(room.BlankWordMode)))
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "显示 IP (ip)：", sprintPropsState(room.EnableShowIP)))
	default:
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "癞子模式 (lz)：", sprintPropsState(room.EnableLaiZi)))
		buf.WriteString(fmt.Sprintf("%-18s%-5v  %-18s%-5v\n", "不洗牌 (ds)：", sprintPropsState(room.EnableDontShuffle), "技能模式 (sk)：", sprintPropsState(room.EnableSkill)))
		buf.WriteString(fmt.Sprintf("%-18s%-5v  %-18s%-5v\n", "人数上限 (pn)：", room.MaxPlayers, "聊天 (ct)：", sprintPropsState(room.EnableChat)))
		buf.WriteString(fmt.Sprintf("%-18s%-5v\n", "显示 IP (ip)：", sprintPropsState(room.EnableShowIP)))
		pwd := room.Password
		if pwd != "" {
			if room.Creator != currPlayer.ID {
				pwd = "********"
			}
		} else {
			pwd = "off"
		}
		buf.WriteString(fmt.Sprintf("%-18s%-20v\n", "房间密码 (pwd)：", pwd))
	}
	_ = currPlayer.WriteString(buf.String())
}

func sprintPropsState(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func maskIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return parts[0] + "." + parts[1] + ".*.*"
	}
	return "*.*.*.*"
}
