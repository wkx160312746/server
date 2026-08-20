package game

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ratel-online/core/util/rand"
	"github.com/ratel-online/server/rule"

	"github.com/ratel-online/core/log"
	modelx "github.com/ratel-online/core/model"
	"github.com/ratel-online/core/util/poker"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
	"github.com/ratel-online/server/skill"
)

type Game struct{}

var (
	stateRob       = 1
	statePlay      = 2
	stateReset     = 3
	stateWaiting   = 4
	stateFirstCard = 5
	stateTakeCard  = 6
)

func (g *Game) Next(player *database.Player) (consts.StateID, error) {
	room := database.GetRoom(player.RoomID)
	if room == nil {
		return 0, player.WriteError(consts.ErrorsExist)
	}
	game := room.Game.(*database.Game)
	buf := bytes.Buffer{}
	if game.Room.EnableLaiZi {
		if game.Room.EnableSkill {
			game.Pokers[player.ID].SetOaa(game.Universals...)
			buf.WriteString(fmt.Sprintf("游戏开始！本局癞子牌：%s %s\n", poker.GetDesc(game.Universals[0]), poker.GetDesc(game.Universals[1])))
		} else {
			game.Pokers[player.ID].SetOaa(game.Universals[0])
			buf.WriteString(fmt.Sprintf("游戏开始！首张癞子牌：%s\n", poker.GetDesc(game.Universals[0])))
		}
		game.Pokers[player.ID].SortByOaaValue()
	} else {
		buf.WriteString("游戏开始！\n")
	}
	if game.Room.EnableSkill {
		buf.WriteString(fmt.Sprintf("你获得的技能：%s\n", skill.Skills[consts.SkillID(game.Skills[player.ID])].Name()))
	}
	buf.WriteString(fmt.Sprintf("你的手牌：%s\n", game.Pokers[player.ID].String()))
	_ = player.WriteString(buf.String())
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[Game.Next] Player %d (Room %d) loop count: %d, room.State: %d\n", player.ID, player.RoomID, loopCount, room.State)
		}
		if room.State == consts.RoomStateWaiting {
			log.Infof("[Game.Next] Player %d exiting, room state changed to waiting, loop count: %d\n", player.ID, loopCount)
			return consts.StateWaiting, nil
		}
		log.Infof("[Game.Next] Player %d waiting for state, loop count: %d\n", player.ID, loopCount)
		state := <-game.States[player.ID]
		switch state {
		case stateRob:
			if !game.Room.EnableLandlord {
				// reset all players group
				for i, id := range game.Players {
					game.Groups[id] = i
					if game.Room.EnableLaiZi {
						game.Pokers[id].SetOaa(game.Universals...)
						game.Pokers[id].SortByOaaValue()
					}
				}
				game.States[player.ID] <- statePlay
			} else {
				err := handleRob(player, game)
				if err != nil {
					log.Error(err)
					return 0, err
				}
			}
		case stateReset:
			if player.ID == room.Creator {
				game.States[game.Players[rand.Intn(len(game.States))]] <- stateRob
			}
			return 0, nil
		case statePlay:
			err := handlePlay(player, game)
			if err != nil {
				log.Error(err)
				return 0, err
			}
		case stateWaiting:
			return consts.StateWaiting, nil
		default:
			return 0, consts.ErrorsChanClosed
		}
	}
}

func (*Game) Exit(player *database.Player) consts.StateID {
	return consts.StateHome
}

func handleRob(player *database.Player, game *database.Game) error {
	if game.FirstPlayer == player.ID && !game.FinalRob {
		if game.FirstRob == 0 {
			err := resetGame(game)
			if err != nil {
				log.Error(err)
				return err
			}
			database.Broadcast(player.RoomID, "所有玩家都放弃抢地主，正在重新发牌……\n")
			for _, playerId := range game.Players {
				game.States[playerId] <- stateReset
			}
		} else if game.FirstRob == game.LastRob {
			landlord := database.GetPlayer(game.LastRob)
			game.FirstPlayer = landlord.ID
			game.LastPlayer = landlord.ID
			game.Groups[landlord.ID] = 1
			game.Pokers[landlord.ID] = append(game.Pokers[landlord.ID], game.Additional...)
			game.Pokers[landlord.ID].SortByOaaValue()

			buf := bytes.Buffer{}
			if game.Room.EnableLaiZi {
				buf.WriteString(fmt.Sprintf("%s 成为地主，获得底牌：%s，最终癞子牌：%s\n", landlord.Name, game.Additional.String(), poker.GetDesc(game.Universals[1])))
				for _, pokers := range game.Pokers {
					pokers.SetOaa(game.Universals...)
					pokers.SortByOaaValue()
				}
			} else {
				buf.WriteString(fmt.Sprintf("%s 成为地主，获得底牌：%s\n", landlord.Name, game.Additional.String()))
			}
			database.Broadcast(player.RoomID, buf.String())
			game.States[landlord.ID] <- statePlay
		} else {
			game.FinalRob = true
			game.States[game.FirstRob] <- stateRob
		}
		return nil
	}
	if game.FirstPlayer == 0 {
		game.FirstPlayer = player.ID
		database.Broadcast(player.RoomID, fmt.Sprintf("轮到 %s 抢地主。\n", player.Name), player.ID)
	}

	timeout := consts.RobTimeout
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[handleRob] Player %d (Room %d) loop count: %d, timeout: %v, FirstRob: %d, LastRob: %d\n", player.ID, player.RoomID, loopCount, timeout, game.FirstRob, game.LastRob)
		}
		before := time.Now().Unix()
		_ = player.WriteString("是否抢地主？请输入 y 或 n。\n")
		ans, err := player.AskForString(timeout)
		if err != nil && err != consts.ErrorsExist {
			ans = "n"
		}
		timeout -= time.Second * time.Duration(time.Now().Unix()-before)
		ans = strings.ToLower(ans)
		if ans == "y" {
			if game.FirstRob == 0 {
				game.FirstRob = player.ID
			}
			game.LastRob = player.ID
			game.Multiple *= 2
			database.Broadcast(player.RoomID, fmt.Sprintf("%s 选择抢地主。\n", player.Name))
			break
		} else if ans == "n" {
			database.Broadcast(player.RoomID, fmt.Sprintf("%s 选择不抢。\n", player.Name))
			break
		} else {
			_ = player.WriteError(consts.ErrorsInputInvalid)
			continue
		}
	}
	if game.FinalRob {
		game.FinalRob = false
		game.FirstRob = game.LastRob
		game.States[game.FirstPlayer] <- stateRob
	} else {
		game.States[game.NextPlayer(player.ID)] <- stateRob
	}
	return nil
}

func playing(player *database.Player, game *database.Game, master bool, playTimes int) error {
	timeout := game.PlayTimeOut[player.ID]
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[playing] Player %d (Room %d) loop count: %d, master: %v, playTimes: %d, timeout: %v\n", player.ID, player.RoomID, loopCount, master, playTimes, timeout)
		}
		buf := bytes.Buffer{}
		buf.WriteString("\n")
		if !master && len(game.LastPokers) > 0 {
			buf.WriteString(fmt.Sprintf("上家：%s（%s），出牌：%s\n", database.GetPlayer(game.LastPlayer).Name, game.Team(game.LastPlayer), game.LastPokers.String()))
		}
		buf.WriteString(fmt.Sprintf("剩余时间：%d 秒，你的手牌：%s\n", int(timeout.Seconds()), game.Pokers[player.ID].String()))
		_ = player.WriteString(buf.String())
		before := time.Now().Unix()
		pokers := game.Pokers[player.ID]
		ans, err := player.AskForString(timeout)
		if err != nil {
			if master {
				ans = poker.GetAlias(pokers[0].Key)
			} else {
				ans = "p"
			}
		} else {
			timeout -= time.Second * time.Duration(time.Now().Unix()-before)
		}
		ans = strings.ToLower(ans)
		if ans == "" {
			_ = player.WriteString(fmt.Sprintf("%s\n", consts.ErrorsPokersFacesInvalid.Error()))
			continue
		} else if ans == "ls" || ans == "v" {
			viewGame(game, player)
			continue
		} else if ans == "p" || ans == "pass" {
			if master {
				_ = player.WriteError(consts.ErrorsHaveToPlay)
				continue
			} else {
				nextPlayer := database.GetPlayer(game.NextPlayer(player.ID))
				database.Broadcast(player.RoomID, fmt.Sprintf("%s 选择不出，下一位：%s。\n", player.Name, nextPlayer.Name))
				game.States[nextPlayer.ID] <- statePlay
				return nil
			}
		}
		normalPokers := map[int]modelx.Pokers{}
		universalPokers := make(modelx.Pokers, 0)
		realSellKeys := make([]int, 0)
		for _, v := range pokers {
			if v.Oaa {
				universalPokers = append(universalPokers, v)
			} else {
				normalPokers[v.Key] = append(normalPokers[v.Key], v)
			}
		}
		sells := make(modelx.Pokers, 0)
		invalid := false
		for _, alias := range ans {
			key := poker.GetKey(string(alias))
			if key == 0 {
				invalid = true
				break
			}
			if len(normalPokers[key]) == 0 {
				if key == 14 || key == 15 || len(universalPokers) == 0 {
					invalid = true
					break
				}
				realSellKeys = append(realSellKeys, universalPokers[0].Key)
				universalPokers[0].Key = key
				universalPokers[0].Desc = poker.GetDesc(key)
				universalPokers[0].Val = game.Rules.Value(key)
				sells = append(sells, universalPokers[0])
				universalPokers = universalPokers[1:]
			} else {
				realSellKeys = append(realSellKeys, key)
				sells = append(sells, normalPokers[key][len(normalPokers[key])-1])
				normalPokers[key] = normalPokers[key][:len(normalPokers[key])-1]
			}
		}
		facesArr := poker.ParseFaces(sells, game.Rules)
		if len(facesArr) == 0 {
			invalid = true
		}
		//聊天開啓才能說話
		if invalid {
			if game.Room.EnableChat {
				database.BroadcastChat(player, fmt.Sprintf("%s [%s] 说：%s\n", player.Name, database.RoleName(player.Role), ans))
				continue
			} else {
				_ = player.WriteString(fmt.Sprintf("%s\n", consts.ErrorsChatUnopened.Error()))
				continue
			}
		}
		lastFaces := game.LastFaces
		if !master && lastFaces != nil {
			if isMax(game, *lastFaces) {
				_ = player.WriteString(fmt.Sprintf("%s\n", consts.ErrorsPokersFacesInvalid.Error()))
				continue
			}
			access := false
			for _, faces := range facesArr {
				if isMax(game, faces) || faces.Compare(*lastFaces) {
					access = true
					lastFaces = &faces
					break
				}
			}
			if !access {
				_ = player.WriteString(fmt.Sprintf("%s\n", consts.ErrorsPokersFacesInvalid.Error()))
				continue
			}
		} else {
			lastFaces = &facesArr[0]
		}
		for _, key := range realSellKeys {
			game.Mnemonic[key]--
		}
		pokers = make(modelx.Pokers, 0)
		for _, curr := range normalPokers {
			pokers = append(pokers, curr...)
		}
		pokers = append(pokers, universalPokers...)
		pokers.SortByOaaValue()
		game.Pokers[player.ID] = pokers
		game.LastPlayer = player.ID
		game.LastFaces = lastFaces
		game.LastPokers = sells
		game.Discards = append(game.Discards, sells...)
		if len(pokers) == 0 {
			database.Broadcast(player.RoomID, fmt.Sprintf("%s 打出 %s，赢得本局！\n", player.Name, sells.OaaString()))
			room := database.GetRoom(player.RoomID)
			if room != nil {
				room.Game = nil
				room.State = consts.RoomStateWaiting
			}
			for _, playerId := range game.Players {
				game.States[playerId] <- stateWaiting
			}
			return nil
		}
		if master {
			playTimes--
			if playTimes > 0 {
				database.Broadcast(player.RoomID, fmt.Sprintf("%s 打出 %s。\n", player.Name, sells.OaaString()))
				return playing(player, game, master, playTimes)
			}
		}
		nextPlayer := database.GetPlayer(game.NextPlayer(player.ID))
		database.Broadcast(player.RoomID, fmt.Sprintf("%s 打出 %s，下一位：%s。\n", player.Name, sells.OaaString(), nextPlayer.Name))
		game.States[nextPlayer.ID] <- statePlay
		return nil
	}
}

func handlePlay(player *database.Player, game *database.Game) error {
	master := player.ID == game.LastPlayer || game.LastPlayer == 0
	database.Broadcast(player.RoomID, fmt.Sprintf("轮到 %s 出牌。\n", player.Name))
	if master && game.Room.EnableSkill {
		sk := skill.Skills[consts.SkillID(game.Skills[player.ID])]
		database.Broadcast(player.RoomID, fmt.Sprintf("%s \n", sk.Desc(player)))
		sk.Apply(player, game)
	}
	return playing(player, game, master, game.PlayTimes[player.ID])
}

func InitGame(room *database.Room) (*database.Game, error) {
	rules := rule.LandlordRules
	if !room.EnableLandlord {
		rules = rule.TeamRules
	}

	distributes, decks := poker.Distribute(room.Players, room.EnableDontShuffle, rules)
	players := make([]int64, 0)
	roomPlayers := database.RoomPlayers(room.ID)
	for playerId := range roomPlayers {
		players = append(players, playerId)
	}
	firstOaa := poker.Random(14, 15)
	lastOaa := poker.Random(14, 15, firstOaa)
	states := map[int64]chan int{}
	groups := map[int64]int{}
	pokers := map[int64]modelx.Pokers{}
	skills := map[int64]int{}
	playTimes := map[int64]int{}
	playTimeout := map[int64]time.Duration{}
	mnemonic := map[int]int{
		14: decks,
		15: decks,
	}
	for i := 1; i <= 13; i++ {
		mnemonic[i] = 4 * decks
	}
	for i := range players {
		states[players[i]] = make(chan int, 1)
		groups[players[i]] = 0
		pokers[players[i]] = distributes[i]
		skills[players[i]] = rand.Intn(len(skill.Skills))
		playTimes[players[i]] = 1
		playTimeout[players[i]] = consts.PlayTimeout
	}
	states[players[rand.Intn(len(states))]] <- stateRob
	return &database.Game{
		Room:        room,
		States:      states,
		Players:     players,
		Groups:      groups,
		Pokers:      pokers,
		Additional:  distributes[len(distributes)-1],
		Multiple:    1,
		Universals:  []int{firstOaa, lastOaa},
		Mnemonic:    mnemonic,
		Decks:       decks,
		Skills:      skills,
		PlayTimes:   playTimes,
		PlayTimeOut: playTimeout,
		Rules:       rules,
		Discards:    modelx.Pokers{},
	}, nil
}

func resetGame(game *database.Game) error {
	distributes, decks := poker.Distribute(len(game.Players), game.Room.EnableDontShuffle, game.Rules)
	if len(distributes) != len(game.Players)+1 {
		return consts.ErrorsGamePlayersInvalid
	}
	players := game.Players
	skills := map[int64]int{}
	playTimes := map[int64]int{}
	playTimeout := map[int64]time.Duration{}
	firstOaa := poker.Random(14, 15)
	lastOaa := poker.Random(14, 15, firstOaa)
	for i := range players {
		game.Pokers[players[i]] = distributes[i]
		skills[players[i]] = rand.Intn(len(skill.Skills))
		playTimes[players[i]] = 1
		playTimeout[players[i]] = consts.PlayTimeout
	}
	game.Groups = map[int64]int{}
	game.FirstPlayer = 0
	game.LastPlayer = 0
	game.FirstRob = 0
	game.LastRob = 0
	game.Additional = distributes[len(distributes)-1]
	game.FinalRob = false
	game.Multiple = 1
	game.Universals = []int{firstOaa, lastOaa}
	game.Decks = decks
	game.Skills = skills
	game.PlayTimes = playTimes
	game.PlayTimeOut = playTimeout
	game.Discards = modelx.Pokers{}
	return nil
}

func viewGame(game *database.Game, currPlayer *database.Player) {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("%-20s%-10s%-10s\n", "玩家", "手牌数", "身份"))
	for _, id := range game.Players {
		player := database.GetPlayer(id)
		flag := ""
		if id == currPlayer.ID {
			flag = "*"
		}
		buf.WriteString(fmt.Sprintf("%-20s%-10d%-10s\n", player.Name+flag, len(game.Pokers[id]), game.Team(id)))
	}
	currKeys := map[int]int{}
	for _, currPoker := range game.Pokers[currPlayer.ID] {
		currKeys[currPoker.Key]++
	}
	buf.WriteString("牌面：")
	for _, i := range consts.MnemonicSorted {
		buf.WriteString(poker.GetDesc(i) + "  ")
	}
	buf.WriteString("\n剩余：")
	for _, i := range consts.MnemonicSorted {
		buf.WriteString(strconv.Itoa(game.Mnemonic[i]-currKeys[i]) + "  ")
		if i == 10 {
			buf.WriteString(" ")
		}
	}
	if game.Room.EnableLaiZi {
		buf.WriteString("\n本局癞子牌：")
		for _, key := range game.Universals {
			buf.WriteString(poker.GetDesc(key) + " ")
		}
	}
	buf.WriteString("\n")
	_ = currPlayer.WriteString(buf.String())
}

func isMax(game *database.Game, faces modelx.Faces) bool {
	if game.Decks == 1 && len(faces.Keys) == 2 {
		if (faces.Keys[0] == 14 && faces.Keys[1] == 15) || (faces.Keys[0] == 15 && faces.Keys[1] == 14) {
			return true
		}
	}
	return false
}
