package consts

import (
	"time"

	"github.com/ratel-online/core/consts"
)

type StateID int

const (
	_ StateID = iota
	StateWelcome
	StateHome
	StateJoin
	StateCreate
	StateWaiting
	StateGame
	StateRunFastGame
	StateUnoGame
	StateMahjongGame
	StateTexasGame
	StateLiarGame
	StateUndercoverGame
)

type SkillID int

const (
	_ SkillID = iota - 1
	SkillWYSS
	SkillHYJJ
	SkillGHJM
	SkillPFCZ
	SkillDHXJ
	SkillLJFZ
	SkillZWZB
	SkillSKLF
	Skill996
	SkillTZJW
)

const (
	IsStart = consts.IsStart
	IsStop  = consts.IsStop

	MinPlayers = 3
	// MaxPlayers https://github.com/ratel-online/server/issues/14 小鄧修改
	MaxPlayers = 3

	RoomStateWaiting = 1
	RoomStateRunning = 2

	GameTypeClassic    = 1
	GameTypeLaiZi      = 2
	GameTypeSkill      = 3
	GameTypeRunFast    = 4
	GameTypeTexas      = 5
	GameTypeMahjong    = 6
	GameTypeLiar       = 7
	GameTypeUno        = 8
	GameTypeUndercover = 9

	RobTimeout         = 20 * time.Second
	PlayTimeout        = 40 * time.Second
	PlayMahjongTimeout = 30 * time.Second
	BetTimeout         = 60 * time.Second
)

// Room properties.
const (
	RoomPropsDotShuffle    = "ds"
	RoomPropsLaiZi         = "lz"
	RoomPropsSkill         = "sk"
	RoomPropsPassword      = "pwd"
	RoomPropsPlayerNum     = "pn"
	RoomPropsChat          = "ct"
	RoomPropsShowIP        = "ip"
	RoomPropsJokerAsTarget = "jt"
	RoomPropsUndercoverNum = "ucn" // 卧底数量
	RoomPropsBlankWordMode = "bwm" // 空白词模式
)

var MnemonicSorted = []int{15, 14, 2, 1, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3}

var RunFastMnemonicSorted = []int{2, 1, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3}

type Error struct {
	Code int
	Msg  string
	Exit bool
}

func (e Error) Error() string {
	return e.Msg
}

func NewErr(code int, exit bool, msg string) Error {
	return Error{Code: code, Exit: exit, Msg: msg}
}

var (
	ErrorsExist                   = NewErr(1, true, "资源不存在。")
	ErrorsChanClosed              = NewErr(1, true, "消息通道已关闭。")
	ErrorsTimeout                 = NewErr(1, false, "操作超时。")
	ErrorsInputInvalid            = NewErr(1, false, "输入无效。")
	ErrorsChatUnopened            = NewErr(1, false, "房间聊天已关闭。")
	ErrorsChatUnopenedDuringGame  = NewErr(1, false, "游戏进行中不能聊天。")
	ErrorsAuthFail                = NewErr(1, true, "认证失败。")
	ErrorsRoomInvalid             = NewErr(1, true, "房间不存在或已失效。")
	ErrorsGameTypeInvalid         = NewErr(1, false, "游戏类型无效。")
	ErrorsRoomPlayersIsFull       = NewErr(1, false, "房间人数已满。")
	ErrorsRoomPassword            = NewErr(1, false, "房间密码错误。")
	ErrorsJoinFailForRoomRunning  = NewErr(1, false, "加入失败，房间正在游戏中。")
	ErrorsJoinFailForKicked       = NewErr(1, false, "加入失败，你已被该房间踢出。")
	ErrorsGamePlayersInvalid      = NewErr(1, false, "当前玩家人数不符合开局要求。")
	ErrorsPokersFacesInvalid      = NewErr(1, false, "牌型无效。")
	ErrorsHaveToPlay              = NewErr(1, false, "当前回合必须出牌。")
	ErrorsMustHaveToPlay          = NewErr(1, false, "存在可以压过上家的牌，必须出牌。")
	ErrorsEndToPlay               = NewErr(1, false, "该牌型只能在最后一手打出。")
	ErrorsUnknownTexasRound       = NewErr(1, false, "未知的德州扑克回合。")
	ErrorsGamePlayersInsufficient = NewErr(1, false, "玩家人数不足，无法开始游戏。")
	ErrorsCannotKickYourself      = NewErr(1, false, "不能踢出自己。")
	ErrorsPlayerNotInRoom         = NewErr(1, true, "玩家不在房间中。")
	GameTypes                     = map[int]string{
		GameTypeClassic:    "斗地主",
		GameTypeLaiZi:      "斗地主-癞子版",
		GameTypeSkill:      "斗地主-大招版",
		GameTypeRunFast:    "跑得快",
		GameTypeTexas:      "德州扑克",
		GameTypeMahjong:    "麻将",
		GameTypeLiar:       "骗子酒馆",
		GameTypeUndercover: "谁是卧底",
	}
	GameTypesIds = []int{
		GameTypeClassic,
		GameTypeLaiZi,
		GameTypeSkill,
		GameTypeRunFast,
		GameTypeTexas,
		GameTypeMahjong,
		GameTypeLiar,
		GameTypeUndercover,
	}
	RoomStates = map[int]string{
		RoomStateWaiting: "等待中",
		RoomStateRunning: "游戏中",
	}
)
