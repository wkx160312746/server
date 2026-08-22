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
	reads  chan *protocol.Packet
	writes []protocol.Packet
}

func (t *reconnectTestTransport) Read() (*protocol.Packet, error) {
	if t.reads == nil {
		return nil, errors.New("not implemented")
	}
	packet, ok := <-t.reads
	if !ok {
		return nil, errors.New("connection closed")
	}
	return packet, nil
}

func (t *reconnectTestTransport) Write(packet protocol.Packet) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.writes = append(t.writes, packet)
	return nil
}

func (t *reconnectTestTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *reconnectTestTransport) IP() string { return t.ip }

func (t *reconnectTestTransport) lastWrite() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.writes) == 0 {
		return ""
	}
	return t.writes[len(t.writes)-1].String()
}

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

func TestRestoreInteractionStateAfterReconnect(t *testing.T) {
	firstTransport := &reconnectTestTransport{ip: "first"}
	firstConn := network.Wrapper(firstTransport)
	player := &Player{ID: 42, Name: "alice"}
	player.Conn(firstConn)
	player.StartTransaction()

	if _, ok := player.Disconnect(firstConn, time.Minute); !ok {
		t.Fatal("disconnect failed")
	}
	secondTransport := &reconnectTestTransport{ip: "second"}
	if !player.Resume(network.Wrapper(secondTransport)) {
		t.Fatal("resume failed")
	}
	if err := player.RestoreInteractionState(); err != nil {
		t.Fatal(err)
	}
	if got := secondTransport.lastWrite(); got != consts.IsStart {
		t.Fatalf("restored interaction state = %q, want %q", got, consts.IsStart)
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

func TestTexasPlayerCanChatOutsideBettingTurn(t *testing.T) {
	senderTransport := &reconnectTestTransport{ip: "sender", reads: make(chan *protocol.Packet, 1)}
	recipientTransport := &reconnectTestTransport{ip: "recipient"}
	senderConn := network.Wrapper(senderTransport)
	recipientConn := network.Wrapper(recipientTransport)
	senderSession := time.Now().UnixNano()
	recipientSession := senderSession + 1

	sender, _, err := Connected(senderConn, &model.AuthInfo{ID: senderSession, Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	recipient, _, err := Connected(recipientConn, &model.AuthInfo{ID: recipientSession, Name: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	room := CreateRoom(sender.ID, consts.GameTypeTexas)
	defer func() {
		deleteRoom(room)
		players.Del(sender.ID)
		players.Del(recipient.ID)
		sessionPlayers.Del(senderSession)
		sessionPlayers.Del(recipientSession)
		connPlayers.Del(senderConn.ID())
		connPlayers.Del(recipientConn.ID())
	}()
	if err := JoinRoom(room.ID, sender.ID); err != nil {
		t.Fatal(err)
	}
	if err := JoinRoom(room.ID, recipient.ID); err != nil {
		t.Fatal(err)
	}
	room.State = consts.RoomStateRunning
	sender.State(consts.StateTexasGame)

	listenDone := make(chan error, 1)
	go func() {
		listenDone <- sender.Listening(senderConn)
	}()
	senderTransport.reads <- &protocol.Packet{Body: []byte("大家好")}

	want := ">> alice [房主] 说：大家好\n"
	deadline := time.Now().Add(time.Second)
	for recipientTransport.lastWrite() != want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := recipientTransport.lastWrite(); got != want {
		t.Fatalf("recipient message from idle WebSocket input = %q, want %q", got, want)
	}
	if got := len(sender.data); got != 0 {
		t.Fatalf("idle chat entered the betting input channel, queued packets = %d", got)
	}

	close(senderTransport.reads)
	select {
	case <-listenDone:
	case <-time.After(time.Second):
		t.Fatal("listener did not stop after transport closed")
	}
}
