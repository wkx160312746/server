package database

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ratel-online/core/model"
	"github.com/ratel-online/core/network"
	"github.com/ratel-online/core/protocol"
	"github.com/ratel-online/server/consts"
)

type reconnectTestTransport struct {
	mu     sync.Mutex
	closed bool
	ip     string
}

func (*reconnectTestTransport) Read() (*protocol.Packet, error) {
	return nil, errors.New("not implemented")
}

func (*reconnectTestTransport) Write(protocol.Packet) error { return nil }

func (t *reconnectTestTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *reconnectTestTransport) IP() string { return t.ip }

func TestConnectedResumesOfflinePlayer(t *testing.T) {
	sessionID := time.Now().UnixNano()
	firstConn := network.Wrapper(&reconnectTestTransport{ip: "first"})
	auth := &model.AuthInfo{ID: sessionID, Name: "alice"}

	player, resumed, err := Connected(firstConn, auth)
	if err != nil || resumed {
		t.Fatalf("first connection = (%v, %v), want a new player", resumed, err)
	}
	if player.ID == sessionID {
		t.Fatal("public player ID exposes the private session ID")
	}
	defer players.Del(player.ID)
	defer sessionPlayers.Del(sessionID)
	defer connPlayers.Del(firstConn.ID())

	epoch, ok := player.Disconnect(firstConn, time.Minute)
	if !ok || epoch == 0 {
		t.Fatal("disconnect did not create a reconnect window")
	}

	wrongNameConn := network.Wrapper(&reconnectTestTransport{ip: "wrong-name"})
	_, _, err = Connected(wrongNameConn, &model.AuthInfo{ID: sessionID, Name: "mallory"})
	if !errors.Is(err, consts.ErrorsAuthFail) {
		t.Fatalf("name mismatch error = %v, want authentication failure", err)
	}

	secondConn := network.Wrapper(&reconnectTestTransport{ip: "second"})
	resumedPlayer, resumed, err := Connected(secondConn, auth)
	if err != nil || !resumed {
		t.Fatalf("reconnection = (%v, %v), want resumed player", resumed, err)
	}
	defer connPlayers.Del(secondConn.ID())
	if resumedPlayer != player {
		t.Fatal("reconnection created a different player")
	}
	if resumedPlayer.ID != firstConn.ID() {
		t.Fatalf("public player ID changed from %d to %d", firstConn.ID(), resumedPlayer.ID)
	}
	if player.IP != "second" || !player.IsOnline() {
		t.Fatalf("resumed player = (ip %q, online %v)", player.IP, player.IsOnline())
	}
	if player.Expire(epoch) {
		t.Fatal("stale reconnect timer expired an active player")
	}
}

func TestPlayerCannotResumeAfterGracePeriod(t *testing.T) {
	firstConn := network.Wrapper(&reconnectTestTransport{ip: "first"})
	player := &Player{ID: 42, Name: "alice"}
	player.Conn(firstConn)

	epoch, ok := player.Disconnect(firstConn, time.Nanosecond)
	if !ok {
		t.Fatal("disconnect failed")
	}
	time.Sleep(time.Millisecond)
	if !player.Expire(epoch) {
		t.Fatal("player did not expire")
	}
	if player.Resume(network.Wrapper(&reconnectTestTransport{ip: "second"})) {
		t.Fatal("expired player resumed")
	}
}

func TestDeletePlayerSessionKeepsReplacement(t *testing.T) {
	const sessionID int64 = 99112233
	oldPlayer := &Player{sessionID: sessionID}
	replacement := &Player{sessionID: sessionID}
	sessionPlayers.Set(sessionID, replacement)
	defer sessionPlayers.Del(sessionID)

	deletePlayerSession(oldPlayer)
	value, ok := sessionPlayers.Get(sessionID)
	if !ok || value.(*Player) != replacement {
		t.Fatal("cleaning up an expired player removed its replacement session")
	}
}
