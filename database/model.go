package database

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ratel-online/core/log"
	"github.com/ratel-online/core/model"
	"github.com/ratel-online/core/network"
	"github.com/ratel-online/core/protocol"
	"github.com/ratel-online/core/util/arrays"
	"github.com/ratel-online/core/util/json"
	"github.com/ratel-online/core/util/poker"
	"github.com/ratel-online/server/consts"
)

const initialRune = 'A'

var errPlayerOffline = errors.New("player is offline")

type runeSequence struct {
	currentRune rune
}

func (s *runeSequence) next() rune {
	if s.currentRune == 0 {
		s.currentRune = initialRune
	}
	currentRune := s.currentRune
	s.currentRune++
	return currentRune
}

type Role string

const (
	RolePlayer    Role = "player"
	RoleOwner     Role = "owner"
	RoleSpectator Role = "spectator"
)

func RoleName(role Role) string {
	switch role {
	case RoleOwner:
		return "房主"
	case RolePlayer:
		return "玩家"
	case RoleSpectator:
		return "观战者"
	default:
		return string(role)
	}
}

type Player struct {
	ID     int64  `json:"id"`
	IP     string `json:"ip"`
	Name   string `json:"name"`
	Mode   int    `json:"mode"`
	Type   int    `json:"type"`
	Amount uint   `json:"amount"`
	RoomID int64  `json:"roomId"`
	Role   Role   `json:"role"`

	connectionMu    sync.RWMutex
	writeMu         sync.Mutex
	sessionID       int64
	conn            *network.Conn
	connectionEpoch uint64
	reconnectUntil  time.Time
	done            chan struct{}
	data            chan *protocol.Packet
	read            bool
	state           consts.StateID
	online          bool
	expired         bool
}

func (p *Player) Write(bytes []byte) error {
	return p.writePacket(protocol.Packet{
		Body: bytes,
	})
}

func (p *Player) IsOnline() bool {
	p.connectionMu.RLock()
	defer p.connectionMu.RUnlock()
	return p.online
}

func (p *Player) IsExpired() bool {
	p.connectionMu.RLock()
	defer p.connectionMu.RUnlock()
	return p.expired
}

func (p *Player) IsRecoverable() bool {
	p.connectionMu.RLock()
	defer p.connectionMu.RUnlock()
	return !p.online && !p.expired && time.Now().Before(p.reconnectUntil)
}

func (p *Player) Listening(conn *network.Conn) error {
	loopCount := 0
	for {
		loopCount++
		if loopCount%1000 == 0 {
			log.Infof("[Player.Listening] Player %d loop count: %d, online: %v\n", p.ID, loopCount, p.IsOnline())
		}
		pack, err := conn.Read()
		if err != nil {
			log.Error(err)
			return err
		}
		p.connectionMu.RLock()
		isCurrentConnection := p.conn == conn && p.online && !p.expired
		reading := p.read
		data := p.data
		p.connectionMu.RUnlock()
		if isCurrentConnection && reading {
			data <- pack
		}
	}
}

// 向客户端发生消息
func (p *Player) WriteString(data string) error {
	return p.writePacket(protocol.Packet{
		Body: []byte(data),
	})
}

func (p *Player) WriteObject(data interface{}) error {
	return p.writePacket(protocol.Packet{
		Body: json.Marshal(data),
	})
}

func (p *Player) WriteError(err error) error {
	if err == consts.ErrorsExist {
		return err
	}
	return p.writePacket(protocol.Packet{
		Body: []byte(err.Error() + "\n"),
	})
}

func (p *Player) writePacket(packet protocol.Packet) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	p.connectionMu.RLock()
	conn := p.conn
	online := p.online && !p.expired
	p.connectionMu.RUnlock()
	if !online || conn == nil {
		return errPlayerOffline
	}
	return conn.Write(packet)
}

func (p *Player) AskForPacket(timeout ...time.Duration) (*protocol.Packet, error) {
	p.StartTransaction()
	defer p.StopTransaction()
	return p.askForPacket(timeout...)
}

func (p *Player) askForPacket(timeout ...time.Duration) (*protocol.Packet, error) {
	var packet *protocol.Packet
	if len(timeout) > 0 {
		select {
		case packet = <-p.data:
		case <-p.done:
			return nil, consts.ErrorsChanClosed
		case <-time.After(timeout[0]):
			return nil, consts.ErrorsTimeout
		}
	} else {
		select {
		case packet = <-p.data:
		case <-p.done:
			return nil, consts.ErrorsChanClosed
		}
	}
	if packet == nil {
		return nil, consts.ErrorsChanClosed
	}
	single := strings.ToLower(packet.String())
	if single == "exit" || single == "e" {
		return nil, consts.ErrorsExist
	}
	return packet, nil
}

func (p *Player) AskForInt(timeout ...time.Duration) (int, error) {
	packet, err := p.AskForPacket(timeout...)
	if err != nil {
		return 0, err
	}
	return packet.Int()
}

func (p *Player) AskForInt64(timeout ...time.Duration) (int64, error) {
	packet, err := p.AskForPacket(timeout...)
	if err != nil {
		return 0, err
	}
	return packet.Int64()
}

func (p *Player) AskForString(timeout ...time.Duration) (string, error) {
	packet, err := p.AskForPacket(timeout...)
	if err != nil {
		return "", err
	}
	return packet.String(), nil
}

func (p *Player) AskForStringWithoutTransaction(timeout ...time.Duration) (string, error) {
	packet, err := p.askForPacket(timeout...)
	if err != nil {
		return "", err
	}
	return packet.String(), nil
}

func (p *Player) StartTransaction() {
	p.connectionMu.Lock()
	p.read = true
	p.connectionMu.Unlock()
	_ = p.WriteString(consts.IsStart)
}

func (p *Player) StopTransaction() {
	p.connectionMu.Lock()
	p.read = false
	p.connectionMu.Unlock()
	_ = p.WriteString(consts.IsStop)
}

func (p *Player) State(s consts.StateID) {
	p.state = s
}

func (p *Player) GetState() consts.StateID {
	return p.state
}

func (p *Player) Conn(conn *network.Conn) {
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()
	p.conn = conn
	p.data = make(chan *protocol.Packet, 8)
	p.done = make(chan struct{})
	p.connectionEpoch++
	p.online = true
}

func (p *Player) Resume(conn *network.Conn) bool {
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()
	if p.online || p.expired || time.Now().After(p.reconnectUntil) {
		return false
	}
	p.conn = conn
	p.IP = conn.IP()
	p.online = true
	p.reconnectUntil = time.Time{}
	p.connectionEpoch++
	return true
}

func (p *Player) Disconnect(conn *network.Conn, gracePeriod time.Duration) (uint64, bool) {
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()
	if p.conn != conn || !p.online {
		return 0, false
	}
	p.conn = nil
	p.online = false
	p.reconnectUntil = time.Now().Add(gracePeriod)
	p.connectionEpoch++
	return p.connectionEpoch, true
}

func (p *Player) Expire(epoch uint64) bool {
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()
	if p.connectionEpoch != epoch || p.online || p.expired {
		return false
	}
	p.expired = true
	close(p.done)
	return true
}

func (p *Player) Model() model.Player {
	modelPlayer := model.Player{
		ID:   p.ID,
		Name: p.Name,
	}
	room := getRoom(p.RoomID)
	if room != nil && room.Game != nil {
		game := room.Game.(*Game)
		modelPlayer.Pokers = len(game.Pokers[p.ID])
		modelPlayer.Group = game.Groups[p.ID]
	}
	return modelPlayer
}

func (p *Player) String() string {
	return fmt.Sprintf("%s[%d]", p.Name, p.ID)
}

type RoomGame interface {
	Clean()
}

type Room struct {
	sync.Mutex

	ID                  int64     `json:"id"`
	Type                int       `json:"type"`
	Game                RoomGame  `json:"gameId"`
	State               int       `json:"state"`
	Players             int       `json:"players"`
	Banker              int       `json:"banker"`
	Robots              int       `json:"robots"`
	Creator             int64     `json:"creator"`
	ActiveTime          time.Time `json:"activeTime"`
	MaxPlayers          int       `json:"maxPlayers"`
	Password            string    `json:"password"`
	EnableChat          bool      `json:"enableChat"`
	EnableLaiZi         bool      `json:"enableLaiZi"`
	EnableSkill         bool      `json:"enableSkill"`
	EnableLandlord      bool      `json:"enableLandlord"`
	EnableDontShuffle   bool      `json:"enableDontShuffle"`
	EnableShowIP        bool      `json:"enableShowIP"`
	EnableJokerAsTarget bool      `json:"enableJokerAsTarget"`
	UndercoverNum       int       `json:"undercoverNum"` // 卧底数量
	BlankWordMode       bool      `json:"blankWordMode"` // 空白词模式
}

func (r *Room) Model() model.Room {
	return model.Room{
		ID:        r.ID,
		Type:      r.Type,
		TypeDesc:  consts.GameTypes[r.Type],
		Players:   r.Players,
		State:     r.State,
		StateDesc: consts.RoomStates[r.State],
		Creator:   r.Creator,
	}
}

type Game struct {
	Room        *Room                   `json:"room"`
	Players     []int64                 `json:"players"`
	Groups      map[int64]int           `json:"groups"`
	States      map[int64]chan int      `json:"states"`
	Pokers      map[int64]model.Pokers  `json:"pokers"`
	Universals  []int                   `json:"universals"`
	Decks       int                     `json:"decks"`
	Additional  model.Pokers            `json:"pocket"`
	Multiple    int                     `json:"multiple"`
	FirstPlayer int64                   `json:"firstPlayer"`
	LastPlayer  int64                   `json:"lastPlayer"`
	Robs        []int64                 `json:"robs"`
	FirstRob    int64                   `json:"firstRob"`
	LastRob     int64                   `json:"lastRob"`
	FinalRob    bool                    `json:"finalRob"`
	LastFaces   *model.Faces            `json:"lastFaces"`
	LastPokers  model.Pokers            `json:"lastPokers"`
	Mnemonic    map[int]int             `json:"mnemonic"`
	Skills      map[int64]int           `json:"skills"`
	PlayTimes   map[int64]int           `json:"playTimes"`
	PlayTimeOut map[int64]time.Duration `json:"playTimeOut"`
	Rules       poker.Rules             `json:"rules"`
	Discards    model.Pokers            `json:"discards"`
}

func (game *Game) Clean() {
	if game != nil {
		for _, state := range game.States {
			close(state)
		}
	}
}

func (game *Game) Start() {

}

func (g Game) NextPlayer(curr int64) int64 {
	idx := arrays.IndexOf(g.Players, curr)
	return g.Players[(idx+1)%len(g.Players)]
}

func (g Game) PrevPlayer(curr int64) int64 {
	idx := arrays.IndexOf(g.Players, curr)
	return g.Players[(idx+len(g.Players))%len(g.Players)]
}

func (g Game) IsTeammate(player1, player2 int64) bool {
	return g.Groups[player1] == g.Groups[player2]
}

func (g Game) IsLandlord(playerId int64) bool {
	return g.Groups[playerId] == 1
}

func (g Game) Team(playerId int64) string {
	if !g.Room.EnableLandlord {
		return "队伍" + strconv.Itoa(g.Groups[playerId]+1)
	} else {
		if !g.IsLandlord(playerId) {
			return "农民"
		} else {
			return "地主"
		}
	}
}
