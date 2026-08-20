package render

import (
	"bytes"
	"fmt"
	constx "github.com/ratel-online/core/consts"
	"github.com/ratel-online/core/model"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
)

func Welcome(player *database.Player) error {
	return player.WriteObject(model.Data{
		Code: constx.CodeWelcome,
		Msg:  fmt.Sprintf("你好，%s！欢迎来到 Ratel Online。\n", player.Name),
	})
}

func HomeOptions(player *database.Player) error {
	buf := bytes.Buffer{}
	buf.WriteString("1. 加入房间\n")
	buf.WriteString("2. 创建房间\n")
	return player.WriteObject(model.Options{
		Data: model.Data{
			Code: constx.CodeHomeOptions,
			Msg:  buf.String(),
		},
		Options: []model.Option{
			{ID: 1, Name: "加入房间"},
			{ID: 2, Name: "创建房间"},
		},
	})
}

func GameTypeOptions(player *database.Player) error {
	buf := bytes.Buffer{}
	buf.WriteString("请选择游戏类型：\n")
	options := make([]model.Option, 0)
	for _, id := range consts.GameTypesIds {
		buf.WriteString(fmt.Sprintf("%d.%s\n", id, consts.GameTypes[id]))
		options = append(options, model.Option{ID: id, Name: consts.GameTypes[id]})
	}
	return player.WriteObject(model.Options{
		Data: model.Data{
			Code: constx.CodeGameTypeOptions,
			Msg:  buf.String(),
		},
		Options: options,
	})
}

func RoomList(player *database.Player) error {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("%-10s%-18s%-10s%-10s\n", "房间 ID", "游戏类型", "人数", "状态"))
	for _, room := range database.GetRooms() {
		buf.WriteString(fmt.Sprintf("%-10d%-18s%-10d%-10s\n", room.ID, consts.GameTypes[room.Type], room.Players, consts.RoomStates[room.State]))
	}
	modelRooms := make([]model.Room, 0)
	for _, room := range database.GetRooms() {
		modelRooms = append(modelRooms, room.Model())
	}
	return player.WriteObject(model.RoomList{
		Data: model.Data{
			Code: constx.CodeRoomList,
			Msg:  buf.String(),
		},
		Rooms: modelRooms,
	})
}

func RoomInfo(player *database.Player, room *database.Room) error {
	buf := bytes.Buffer{}

	// 如果游戏正在进行中，显示玩家号码
	if room.Game != nil {
		if undercoverGame, ok := room.Game.(*database.Undercover); ok {
			buf.WriteString(fmt.Sprintf("%-10s%-20s%-10s%-10s\n", "编号", "名称", "积分", "身份"))
			for playerId := range database.RoomPlayers(room.ID) {
				title := "玩家"
				if playerId == room.Creator {
					title = "房主"
				}
				info := database.GetPlayer(playerId)
				playerNum := undercoverGame.PlayerNumbers[playerId]
				buf.WriteString(fmt.Sprintf("%-10d%-20s%-10d%-10s\n", playerNum, info.Name, info.Amount, title))
			}
			return player.WriteString(buf.String())
		}
	}

	// 默认显示（等待状态或其他游戏）
	buf.WriteString(fmt.Sprintf("%-20s%-10s%-10s\n", "名称", "积分", "身份"))
	for playerId := range database.RoomPlayers(room.ID) {
		title := "玩家"
		if playerId == room.Creator {
			title = "房主"
		}
		info := database.GetPlayer(playerId)
		buf.WriteString(fmt.Sprintf("%-20s%-10d%-10s\n", info.Name, info.Amount, title))
	}
	return player.WriteString(buf.String())
}

func Error(player *database.Player, err error) error {
	return player.WriteError(err)
}

func Join(player *database.Player, room *database.Room) {
	database.BroadcastObject(room.ID, model.RoomEvent{
		Data: model.Data{
			Code: constx.CodeRoomEventJoin,
			Msg:  fmt.Sprintf("%s 加入了房间，当前共有 %d 名玩家。\n", player.Name, room.Players),
		},
		Room:   room.Model(),
		Player: player.Model(),
	})
}

func Exit(player *database.Player, room *database.Room) {
	database.BroadcastObject(room.ID, model.RoomEvent{
		Data: model.Data{
			Code: constx.CodeRoomEventExit,
			Msg:  fmt.Sprintf("%s 离开了房间，当前共有 %d 名玩家。\n", player.Name, room.Players),
		},
		Room:   room.Model(),
		Player: player.Model(),
	})
}

func Offline(player *database.Player, room *database.Room) {
	database.BroadcastObject(room.ID, model.RoomEvent{
		Data: model.Data{
			Code: constx.CodeRoomEventOffline,
			Msg:  fmt.Sprintf("%s 已断开连接", player.Name),
		},
		Room:   room.Model(),
		Player: player.Model(),
	})
}

func OwnerChange(player *database.Player, room *database.Room) {
	database.BroadcastObject(room.ID, model.RoomEvent{
		Data: model.Data{
			Code: constx.CodeRoomEventOwnerChange,
			Msg:  fmt.Sprintf("%s 成为新房主。\n", player.Name),
		},
		Room:   room.Model(),
		Player: player.Model(),
	})
}
