package sio

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tomruk/socket.io-go/parser"
)

func TestClientSocketStore(t *testing.T) {
	store := newClientSocketStore()
	manager := NewManager("http://asdf.jkl", nil)
	main := manager.Socket("/", nil).(*clientSocket)

	store.set(main)
	s, ok := store.get("/")
	if !assert.True(t, ok) {
		return
	}
	assert.True(t, main == s)

	foo := manager.Socket("/foo", nil).(*clientSocket)
	store.set(foo)

	sockets := store.getAll()
	if !assert.Equal(t, 2, len(sockets)) {
		return
	}
	assert.Contains(t, sockets, main)
	assert.Contains(t, sockets, foo)
	// We used to this, but maps are not ordered, so we do the above test.
	// assert.True(t, main == sockets[0])
	// assert.True(t, foo == sockets[1])

	store.remove("/foo")
	sockets = store.getAll()
	if !assert.Equal(t, 1, len(sockets)) {
		return
	}
	assert.True(t, main == sockets[0])
}

func TestServerSocketStore(t *testing.T) {
	store := newServerSocketStore()
	server, _, manager := newTestServerAndClient(t, nil, nil)

	var (
		socket   *serverSocket
		socketTW = newTestWaiter(0)
	)

	socketTW.Add(1)
	server.Of("/").OnConnection(func(_socket ServerSocket) {
		fmt.Printf("New connection to `/` with sid: %s\n", _socket.ID())
		_socket.OnError(func(err error) {
			t.Fatal(err)
		})
		socket = _socket.(*serverSocket)
		store.set(socket)
		socketTW.Done()
	})

	manager.Socket("/", nil).Connect()
	timedout := socketTW.WaitTimeout(t, defaultTestWaitTimeout)
	if timedout {
		return
	}

	assert.Equal(t, 1, len(store.socketsByID))
	assert.Equal(t, 1, len(store.socketsByNsp))

	s, ok := store.getByID(socket.ID())
	if !assert.True(t, ok) {
		return
	}
	assert.True(t, socket == s)

	s, ok = store.getByNsp("/")
	if !assert.True(t, ok) {
		return
	}
	assert.True(t, socket == s)

	sockets := store.getAll()
	if !assert.Equal(t, 1, len(sockets)) {
		return
	}
	assert.Contains(t, sockets, socket)

	sockets = store.getAndRemoveAll()
	if !assert.Equal(t, 1, len(sockets)) {
		return
	}
	assert.Contains(t, sockets, socket)
	assert.Equal(t, 0, len(store.socketsByID))
	assert.Equal(t, 0, len(store.socketsByNsp))

	socketTW.Add(1)
	server.Of("/asdf").OnConnection(func(_socket ServerSocket) {
		fmt.Printf("New connection to `/asdf` with sid: %s\n", _socket.ID())
		_socket.OnError(func(err error) {
			t.Fatal(err)
		})
		socket = _socket.(*serverSocket)
		store.set(socket)
		socketTW.Done()
	})

	//manager = NewManager(httpServer.URL, nil)
	manager.Socket("/asdf", nil).Connect()
	timedout = socketTW.WaitTimeout(t, defaultTestWaitTimeout)
	if timedout {
		return
	}

	assert.Equal(t, 1, len(store.socketsByID))
	assert.Equal(t, 1, len(store.socketsByNsp))

	s, ok = store.getByNsp("/asdf")
	if !assert.True(t, ok) {
		return
	}
	assert.True(t, socket == s)

	store.removeByID(socket.ID())
	assert.Equal(t, 0, len(store.socketsByID))
	assert.Equal(t, 0, len(store.socketsByNsp))
}

func TestNamespaceStore(t *testing.T) {
	store := newNspStore()
	server, _, _ := newTestServerAndClient(t, nil, nil)

	main := server.Of("/")
	asdf := server.Of("/asdf")
	store.set(main)
	store.set(asdf)

	assert.Equal(t, 2, store.len())
	n, ok := store.get("/")
	assert.True(t, ok)
	assert.True(t, n == main)

	n, ok = store.get("/asdf")
	assert.True(t, ok)
	assert.True(t, n == asdf)

	store.remove("/asdf")
	n, ok = store.get("/asdf")
	assert.False(t, ok)
	assert.True(t, n == nil)

	n, created := store.getOrCreate("/jkl", server, server.adapterCreator, server.parserCreator)
	assert.True(t, created)
	assert.Equal(t, "/jkl", n.Name())

	n, created = store.getOrCreate("/", server, server.adapterCreator, server.parserCreator)
	assert.False(t, created)
	assert.True(t, n == main)
}

func TestNamespaceSocketStore(t *testing.T) {
	store := newNspSocketStore()
	server, _, manager := newTestServerAndClient(t, nil, nil)
	tw := newTestWaiter(2)

	var (
		main, asdf ServerSocket
	)

	server.Of("/").OnConnection(func(socket ServerSocket) {
		main = socket
		tw.Done()
	})

	server.Of("/asdf").OnConnection(func(socket ServerSocket) {
		asdf = socket
		tw.Done()
	})

	manager.Socket("/", nil).Connect()
	manager.Socket("/asdf", nil).Connect()
	timedout := tw.WaitTimeout(t, defaultTestWaitTimeout)
	if timedout {
		return
	}

	store.set(main)
	store.set(asdf)
	assert.Equal(t, 2, len(store.sockets))

	s, ok := store.get(main.ID())
	assert.True(t, ok)
	assert.True(t, s == main)

	s, ok = store.get(asdf.ID())
	assert.True(t, ok)
	assert.True(t, s == asdf)

	store.remove(asdf.ID())
	s, ok = store.get("/asdf")
	assert.False(t, ok)
	assert.True(t, s == nil)

	sockets := store.getAll()
	assert.Equal(t, 1, len(sockets))
	assert.True(t, sockets[0] == main)

	// There is no such socket.
	ok = store.sendBuffers("", nil)
	assert.False(t, ok)

	tw.Add(1)
	manager.Socket("/", nil).OnEvent("hi", func(message string) {
		assert.Equal(t, "I am Groot", message)
		tw.Done()
	})

	_main := main.(*serverSocket)
	_, buffers := mustCreateEventPacket(_main, "hi", []any{"I am Groot"})
	store.sendBuffers(main.ID(), buffers)

	tw.WaitTimeout(t, defaultTestWaitTimeout)
}

func mustCreateEventPacket(socket *serverSocket, eventName string, _v []any) (header *parser.PacketHeader, buffers [][]byte) {
	header = &parser.PacketHeader{
		Type:      parser.PacketTypeEvent,
		Namespace: socket.nsp.Name(),
	}

	if IsEventReservedForServer(eventName) {
		panic("sio: Emit: attempted to emit a reserved event: `" + eventName + "`")
	}

	v := make([]any, 0, len(_v)+1)
	v = append(v, eventName)
	v = append(v, _v...)

	var err error
	buffers, err = socket.parser.Encode(header, &v)
	if err != nil {
		panic(err)
	}
	return
}