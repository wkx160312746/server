package state

import (
	"bytes"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
)

type home struct{}

func (*home) Next(player *database.Player) (consts.StateID, error) {
	buf := bytes.Buffer{}
	buf.WriteString("1. 加入房间\n")
	buf.WriteString("2. 创建房间\n")
	err := player.WriteString(buf.String())
	if err != nil {
		return 0, player.WriteError(err)
	}
	selected, err := player.AskForInt()
	if err != nil {
		return 0, player.WriteError(err)
	}
	if selected == 1 {
		return consts.StateJoin, nil
	} else if selected == 2 {
		return consts.StateCreate, nil
	}
	return 0, player.WriteError(consts.ErrorsInputInvalid)
}

func (*home) Exit(player *database.Player) consts.StateID {
	return 0
}
