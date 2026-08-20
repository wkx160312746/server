package state

import (
	"bytes"
	"fmt"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
)

type welcome struct{}

func (*welcome) Next(player *database.Player) (consts.StateID, error) {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("你好，%s！欢迎来到 Ratel Online。游戏规则：https://github.com/ratel-online/server/blob/main/README.md\n", player.Name))
	err := player.WriteString(buf.String())
	if err != nil {
		return 0, player.WriteError(err)
	}
	return consts.StateHome, nil
}

func (*welcome) Exit(player *database.Player) consts.StateID {
	return 0
}
