package database

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/feel-easy/uno/card"
	"github.com/feel-easy/uno/card/color"
	"github.com/feel-easy/uno/event"
	"github.com/feel-easy/uno/game"
	"github.com/ratel-online/core/log"
	"github.com/ratel-online/server/consts"
)

type UnoGame struct {
	Room    *Room            `json:"room"`
	Players []int            `json:"players"`
	States  map[int]chan int `json:"states"`
	Game    *game.Game       `json:"game"`
}

func (ug *UnoGame) HavePlay(player *Player) bool {
	for _, id := range ug.Players {
		if id == int(player.ID) && player.online {
			return true
		}
	}
	return false
}

func (un *UnoGame) NeedExit() bool {
	return un.Room.Players <= 1
}

func (un *UnoGame) Clean() {
	if un != nil {
		for _, state := range un.States {
			close(state)
		}
	}
}

type UnoPlayer struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func NewUnoPlayer(p *Player) game.Player {
	return &UnoPlayer{
		ID:   int(p.ID),
		Name: p.Name,
	}
}

func (up *UnoPlayer) PlayerID() int {
	return up.ID
}

func (up *UnoPlayer) NickName() string {
	return up.Name
}

func contains(cards []card.Card, searchedCard card.Card) bool {
	for _, card := range cards {
		if card.Equal(searchedCard) {
			return true
		}
	}
	return false
}

func (up *UnoPlayer) NotifyCardsDrawn(cards []card.Card) {
	p := getPlayer(int64(up.ID))
	getPlayer(p.ID).WriteString(fmt.Sprintf("你抽到了：%s\n", cards))
}

func (up *UnoPlayer) NotifyNoMatchingCardsInHand(lastPlayedCard card.Card, hand []card.Card) {
	p := getPlayer(int64(up.ID))
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("%s，你的手牌中没有可以匹配 %s 的牌。\n", p.Name, lastPlayedCard))
	buf.WriteString(fmt.Sprintf("你的手牌：%s\n", hand))
	getPlayer(p.ID).WriteString(buf.String())
}

func (up *UnoPlayer) OnFirstCardPlayed(payload event.FirstCardPlayedPayload) {
	p := getPlayer(int64(up.ID))
	Broadcast(p.RoomID, fmt.Sprintf("首张牌是：%s\n", payload.Card))
}

func (up *UnoPlayer) OnCardPlayed(payload event.CardPlayedPayload) {
	p := getPlayer(int64(up.ID))
	Broadcast(p.RoomID, fmt.Sprintf("%s 打出 %s。\n", payload.PlayerName, payload.Card))
}

func (up *UnoPlayer) OnColorPicked(payload event.ColorPickedPayload) {
	p := getPlayer(int64(up.ID))
	Broadcast(p.RoomID, fmt.Sprintf("%s 选择了颜色 %s。\n", payload.PlayerName, payload.Color))
}

func (up *UnoPlayer) OnPlayerPassed(payload event.PlayerPassedPayload) {
	p := getPlayer(int64(up.ID))
	Broadcast(p.RoomID, fmt.Sprintf("%s 跳过了本回合。\n", payload.PlayerName))
}

func (up *UnoPlayer) PickColor(gameState game.State) color.Color {
	p := getPlayer(int64(up.ID))
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[UnoPlayer.PickColor] Player %d loop count: %d\n", up.ID, loopCount)
		}
		p = getPlayer(p.ID)
		p.WriteString("请选择颜色（输入英文缩写）：红色(r)、黄色(y)、绿色(g)或蓝色(b)。\n")
		colorName, err := p.AskForString(consts.PlayTimeout)
		if err != nil {
			if err == consts.ErrorsTimeout {
				return color.Red
			}
			p.WriteString(fmt.Sprintf("无法识别颜色 %q，请输入 r、y、g 或 b。\n", colorName))
			continue
		}
		chosenColor, err := color.ByName(strings.ToLower(colorName))
		if err != nil {
			p.WriteString(fmt.Sprintf("无法识别颜色 %q，请输入 r、y、g 或 b。\n", colorName))
			continue
		}
		return chosenColor
	}
}

func (up *UnoPlayer) Play(playableCards []card.Card, gameState game.State) (card.Card, error) {
	p := getPlayer(int64(up.ID))
	Broadcast(p.RoomID, fmt.Sprintf("轮到 %s 出牌。\n", p.Name), p.ID)
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("%s，轮到你出牌了！\n", p.Name))
	buf.WriteString(unoStateText(gameState))
	p.WriteString(buf.String())
	runeSequence := runeSequence{}
	cardOptions := make(map[string]card.Card)
	for _, card := range playableCards {
		label := string(runeSequence.next())
		cardOptions[label] = card
	}
	cardSelectionLines := []string{"请选择要打出的牌（输入对应字母）："}
	for label, card := range cardOptions {
		cardSelectionLines = append(cardSelectionLines, fmt.Sprintf("%s %s", label, card))
	}
	cardSelectionMessage := strings.Join(cardSelectionLines, " \n ") + " \n "
	loopCount := 0
	for {
		loopCount++
		if loopCount%100 == 0 {
			log.Infof("[UnoPlayer.Play] Player %d loop count: %d\n", up.ID, loopCount)
		}
		p = getPlayer(p.ID)
		p.WriteString(cardSelectionMessage)
		selectedLabel, err := p.AskForString(consts.PlayTimeout)
		if err != nil {
			if err == consts.ErrorsTimeout {
				selectedLabel = "A"
			} else {
				return nil, err
			}
		}
		selectedCard, found := cardOptions[strings.ToUpper(selectedLabel)]
		if !found {
			BroadcastChat(p, fmt.Sprintf("%s 说：%s\n", p.Name, selectedLabel))
			continue
		}
		if !contains(playableCards, selectedCard) {
			p.WriteString(fmt.Sprintf("出牌无效：%s 不在 %s 的手牌中。\n", selectedCard, p.Name))
			continue
		}
		return selectedCard, nil
	}
}

func unoStateText(state game.State) string {
	lines := []string{fmt.Sprintf("上一张牌：%s", state.LastPlayedCard)}
	playerStatuses := make([]string, 0, len(state.PlayerSequence))
	for _, playerName := range state.PlayerSequence {
		playerStatuses = append(playerStatuses, fmt.Sprintf("%s（%d 张牌）", playerName, state.PlayerHandCounts[playerName]))
	}
	lines = append(lines, fmt.Sprintf("出牌顺序：%s", strings.Join(playerStatuses, "、")))
	lines = append(lines, fmt.Sprintf("你的手牌：%s\n", state.CurrentPlayerHand))
	return strings.Join(lines, "\n")
}
