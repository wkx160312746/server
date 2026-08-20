package database

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/feel-easy/mahjong/card"
	"github.com/feel-easy/mahjong/consts"
	"github.com/feel-easy/mahjong/event"
	"github.com/feel-easy/mahjong/game"
	"github.com/feel-easy/mahjong/tile"
	"github.com/ratel-online/core/log"
	rconsts "github.com/ratel-online/server/consts"
)

type Mahjong struct {
	Room      *Room            `json:"room"`
	PlayerIDs []int            `json:"playerIds"`
	States    map[int]chan int `json:"states"`
	Game      *game.Game       `json:"game"`
}

func (game *Mahjong) Clean() {
	if game != nil {
		for _, state := range game.States {
			close(state)
		}
	}
}

type OP struct {
	operation int
	tiles     []int
}

func circled(n int) string {
	if n >= 1 && n <= 20 {
		return string(rune(0x2460 + n - 1))
	}
	return strconv.Itoa(n)
}

type MahjongPlayer struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func NewPlayer(user *Player) *MahjongPlayer {
	return &MahjongPlayer{
		ID:   user.ID,
		Name: user.Name,
	}
}

func (p *MahjongPlayer) PlayerID() int {
	return int(p.ID)
}

func (p *MahjongPlayer) NickName() string {
	return p.Name
}

func (mp *MahjongPlayer) OnPlayTile(payload event.PlayTilePayload) {
	p := GetPlayer(mp.ID)
	p.WriteString(fmt.Sprintf("你打出了：%s\n", tile.Tile(payload.Tile)))
	Broadcast(p.RoomID, fmt.Sprintf("%s 打出了 %s。\n", payload.PlayerName, tile.Tile(payload.Tile)), p.ID)
}

func (mp *MahjongPlayer) Take(tiles []int, gameState game.State) (int, []int, error) {
	p := GetPlayer(mp.ID)
	Broadcast(p.RoomID, fmt.Sprintf("轮到 %s 摸牌。\n", p.Name), p.ID)
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("%s，轮到你摸牌了！\n", p.Name))
	buf.WriteString(mahjongStateText(gameState))
	p.WriteString(buf.String())
	askBuf := bytes.Buffer{}
	tileOptions := make(map[string]*OP)
	labelCounter := 1
	if pvs, ok := gameState.SpecialPrivileges[int(p.ID)]; ok {
		for _, pv := range pvs {
			switch pv {
			case consts.GANG:
				askBuf.WriteString("你可以杠牌！\n")
				label := strconv.Itoa(labelCounter)
				ts := []int{gameState.LastPlayedTile, gameState.LastPlayedTile, gameState.LastPlayedTile}
				tileOptions[label] = &OP{
					operation: consts.GANG,
					tiles:     append(ts, gameState.LastPlayedTile),
				}
				askBuf.WriteString(fmt.Sprintf("%s. %s \n", circled(labelCounter), tile.ToTileString(ts)))
				labelCounter++
			case consts.PENG:
				askBuf.WriteString("你可以碰牌！\n")
				label := strconv.Itoa(labelCounter)
				ts := []int{gameState.LastPlayedTile, gameState.LastPlayedTile}
				tileOptions[label] = &OP{
					operation: consts.PENG,
					tiles:     append(ts, gameState.LastPlayedTile),
				}
				askBuf.WriteString(fmt.Sprintf("%s. %s \n", circled(labelCounter), tile.ToTileString(ts)))
				labelCounter++
			case consts.CHI:
				askBuf.WriteString("你可以吃牌！\n")
				for _, ts := range card.CanChiTiles(tiles, gameState.LastPlayedTile) {
					label := strconv.Itoa(labelCounter)
					tileOptions[label] = &OP{
						operation: consts.CHI,
						tiles:     append(ts, gameState.LastPlayedTile),
					}
					askBuf.WriteString(fmt.Sprintf("%s. %s \n", circled(labelCounter), tile.ToTileString(ts)))
					labelCounter++
				}
			}
		}
	}
	label := strconv.Itoa(labelCounter)
	askBuf.WriteString(fmt.Sprintf("%s. 不操作\n", circled(labelCounter)))
	tileOptions[label] = &OP{
		operation: 0,
		tiles:     []int{},
	}
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[MahjongPlayer.Take] Player %d loop count: %d\n", mp.ID, loopCount)
		}
		p = getPlayer(p.ID)
		p.WriteString(askBuf.String())
		selectedLabel, err := p.AskForString(consts.PlayMahjongTimeout)
		if err != nil {
			switch err {
			case rconsts.ErrorsExist:
				p.WriteString("请勿中途退出本局游戏。\n")
				selectedLabel = "E"
			case rconsts.ErrorsTimeout:
				// Default to "no" action (skip peng/chi/gang) on timeout
				selectedLabel = label
			default:
				return 0, nil, err
			}
		}
		selected, found := tileOptions[strings.ToUpper(selectedLabel)]
		if !found {
			BroadcastChat(p, fmt.Sprintf("%s 说：%s\n", p.Name, selectedLabel))
			continue
		}
		return selected.operation, selected.tiles, nil
	}
}

func (mp *MahjongPlayer) Play(tiles []int, gameState game.State) (int, error) {
	p := GetPlayer(mp.ID)
	Broadcast(p.RoomID, fmt.Sprintf("轮到 %s 出牌。\n", p.Name), p.ID)
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("%s，轮到你出牌了！\n", p.Name))
	buf.WriteString(mahjongStateText(gameState))
	p.WriteString(buf.String())
	askBuf := bytes.Buffer{}
	askBuf.WriteString("请选择要打出的牌（输入对应编号）：\n")
	tileOptions := make(map[string]int)
	sort.Ints(tiles)
	for idx, i := range tiles {
		label := strconv.Itoa(idx + 1)
		tileOptions[label] = i
		askBuf.WriteString(fmt.Sprintf("%s. %-6s", circled(idx+1), tile.Tile(i).String()))
		if (idx+1)%6 == 0 {
			askBuf.WriteString("\n")
		} else {
			askBuf.WriteString("  ")
		}
	}
	askBuf.WriteString("\n")
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[MahjongPlayer.Play] Player %d loop count: %d\n", mp.ID, loopCount)
		}
		p = GetPlayer(p.ID)
		p.WriteString(askBuf.String())
		selectedLabel, err := p.AskForString(rconsts.PlayMahjongTimeout)
		if err != nil {
			switch err {
			case rconsts.ErrorsExist:
				p.WriteString("请勿中途退出本局游戏。\n")
				selectedLabel = "E"
			case rconsts.ErrorsTimeout:
				// Default to the last tile (least likely to be useful) on timeout
				selectedLabel = strconv.Itoa(len(tiles))
			default:
				return 0, err
			}
		}
		selectedCard, found := tileOptions[strings.ToUpper(selectedLabel)]
		if !found {
			BroadcastChat(p, fmt.Sprintf("%s 说：%s\n", p.Name, selectedLabel))
			continue
		}
		mp.OnPlayTile(event.PlayTilePayload{
			PlayerName: p.Name,
			Tile:       selectedCard,
		})
		return selectedCard, nil
	}
}

func mahjongStateText(state game.State) string {
	lines := []string{fmt.Sprintf("已打出的牌：%s", tile.ToTileString(state.PlayedTiles))}
	if state.LastPlayer != nil {
		lines = append(lines, fmt.Sprintf("%s 打出了：%s", state.LastPlayer.Name(), tile.Tile(state.LastPlayedTile)))
	}
	hand := append([]int(nil), state.CurrentPlayerHand...)
	if len(hand) > 0 {
		lines = append(lines, fmt.Sprintf("你摸到的牌：%s", tile.Tile(hand[len(hand)-1])))
	}
	sort.Ints(hand)
	lines = append(lines, fmt.Sprintf("你的手牌：%s\n", tile.ToTileString(hand)))
	return strings.Join(lines, "\n")
}
