package state

import (
	"bytes"
	"fmt"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
	"strconv"
)

type join struct{}

func (s *join) Next(player *database.Player) (consts.StateID, error) {
	buf := bytes.Buffer{}
	rooms := database.GetRooms()
	buf.WriteString(fmt.Sprintf("%-10s%-18s%-10s%-10s\n", "房间 ID", "游戏类型", "人数", "状态"))
	for _, room := range rooms {
		pwdFlag := ""
		if room.Password != "" {
			pwdFlag = "*"
		}
		buf.WriteString(fmt.Sprintf("%-10d%-18s%-10d%-10s\n", room.ID, pwdFlag+consts.GameTypes[room.Type], room.Players, consts.RoomStates[room.State]))
	}
	err := player.WriteString(buf.String())
	if err != nil {
		return 0, player.WriteError(err)
	}
	signal, err := player.AskForString()
	if err != nil {
		return 0, player.WriteError(err)
	}
	if isExit(signal) {
		return s.Exit(player), nil
	}
	if isLs(signal) {
		return consts.StateJoin, nil
	}
	roomId, err := strconv.ParseInt(signal, 10, 64)
	if err != nil {
		return 0, player.WriteError(consts.ErrorsRoomInvalid)
	}
	room := database.GetRoom(roomId)
	if room == nil {
		return 0, player.WriteError(consts.ErrorsRoomInvalid)
	}

	//房间存在密码，要求输入密码
	pwd := room.Password
	if pwd != "" {
		err = verifyPassword(player, pwd)
		if err != nil {
			return 0, player.WriteError(err)
		}
	}
	err = database.JoinRoom(roomId, player.ID)
	if err != nil {
		return 0, player.WriteError(err)
	}
	if room.State != consts.RoomStateRunning {
		database.Broadcast(roomId, fmt.Sprintf("%s [%s] 加入了房间，当前共有 %d 名玩家。\n", player.Name, database.RoleName(player.Role), room.Players))
	} else {
		_ = player.WriteString("你已进入正在游戏的房间，将以观战者身份等待本局结束。\n")
	}
	return consts.StateWaiting, nil
}

func (*join) Exit(player *database.Player) consts.StateID {
	return consts.StateHome
}

// 校验密码
func verifyPassword(player *database.Player, pwd string) error {
	err := player.WriteString("请输入房间密码：\n")
	if err != nil {
		return err
	}
	password, err := player.AskForString()
	if err != nil {
		return err
	}
	if password != pwd {
		return consts.ErrorsRoomPassword
	}
	return nil
}
