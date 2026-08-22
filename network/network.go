package network

import (
	"github.com/ratel-online/core/log"
	"github.com/ratel-online/core/model"
	"github.com/ratel-online/core/network"
	"github.com/ratel-online/core/protocol"
	"github.com/ratel-online/core/util/async"
	"github.com/ratel-online/server/consts"
	"github.com/ratel-online/server/database"
	"github.com/ratel-online/server/state"
	"time"
)

// Network is interface of all kinds of network.
type Network interface {
	Serve() error
}

func handle(rwc protocol.ReadWriteCloser) error {
	// 给新进入的用户分配资源
	c := network.Wrapper(rwc)
	defer func() {
		_ = c.Close()
	}()
	log.Info("new player connected! ")
	authInfo, err := loginAuth(c)
	if err != nil {
		_ = c.Write(protocol.ErrorPacket(err))
		return err
	}
	player, resumed, err := database.Connected(c, authInfo)
	if err != nil {
		_ = c.Write(protocol.ErrorPacket(err))
		return err
	}
	log.Infof("player auth accessed, ip %s, %d:%s\n", player.IP, player.ID, authInfo.Name)
	if resumed {
		if player.RoomID > 0 {
			_ = player.WriteString("连接已恢复，你已回到原房间和对局。\n")
		} else {
			_ = player.WriteString("连接已恢复，正在继续之前的会话。\n")
		}
		_ = player.RestoreInteractionState()
	} else {
		go state.Run(player)
	}
	defer database.Disconnected(player, c)
	return player.Listening(c)
}

// 登陆验签
func loginAuth(c *network.Conn) (*model.AuthInfo, error) {
	authChan := make(chan *model.AuthInfo)
	defer close(authChan)
	async.Async(func() {
		packet, err := c.Read()
		if err != nil {
			log.Error(err)
			return
		}
		authInfo := &model.AuthInfo{}
		err = packet.Unmarshal(authInfo)
		if err != nil {
			log.Error(err)
			return
		}
		authChan <- authInfo
	})
	select {
	case authInfo := <-authChan:
		return authInfo, nil
	case <-time.After(3 * time.Second):
		return nil, consts.ErrorsAuthFail
	}
}
