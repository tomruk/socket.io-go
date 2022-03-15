package sio

import (
	"encoding/json"
	"reflect"
	"sync"

	eio "github.com/tomruk/socket.io-go/engine.io"
	eioparser "github.com/tomruk/socket.io-go/engine.io/parser"
	"github.com/tomruk/socket.io-go/parser"
)

type serverSocketStore struct {
	sockets map[string]*serverSocket
	mu      sync.Mutex
}

func newServerSocketStore() *serverSocketStore {
	return &serverSocketStore{
		sockets: make(map[string]*serverSocket),
	}
}

func (s *serverSocketStore) Get(sid string) (ss *serverSocket, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok = s.sockets[sid]
	return
}

func (s *serverSocketStore) GetAll() (sockets []*serverSocket) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sockets = make([]*serverSocket, len(s.sockets))
	i := 0
	for _, ss := range s.sockets {
		sockets[i] = ss
		i++
	}
	return
}

func (s *serverSocketStore) Set(ss *serverSocket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sockets[ss.ID()] = ss
}

func (s *serverSocketStore) Remove(sid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sockets, sid)
}

// This struct represents a connection to the server.
//
// This is the equivalent of the Client class at: https://github.com/socketio/socket.io/blob/4.3.2/lib/client.ts#L21
type serverConn struct {
	id  string
	eio eio.Socket

	server  *Server
	sockets *serverSocketStore
	nsps    *namespaceStore

	// This mutex is used for protecting parser from concurrent calls.
	// Due to the modular and concurrent nature of Engine.IO,
	// we should use a mutex to ensure the Engine.IO doesn't access
	// the parser's Add method from multiple goroutines.
	parserMu sync.Mutex
	parser   parser.Parser
}

// Engine.IO ID
func (c *serverConn) ID() string {
	return c.id
}

func newServerConn(server *Server, _eio eio.Socket, creator parser.Creator) (*serverConn, *eio.Callbacks) {
	c := &serverConn{
		id:  _eio.ID(),
		eio: _eio,

		server:  server,
		sockets: newServerSocketStore(),
		nsps:    newNamespaceStore(),

		parser: creator(),
	}

	callbacks := &eio.Callbacks{
		OnPacket: c.onEIOPacket,
		OnError:  c.onError,
		OnClose:  c.onClose,
	}

	return c, callbacks
}

func (c *serverConn) onEIOPacket(packets ...*eioparser.Packet) {
	c.parserMu.Lock()
	defer c.parserMu.Unlock()

	for _, packet := range packets {
		if packet.Type == eioparser.PacketTypeMessage {
			err := c.parser.Add(packet.Data, c.onFinishEIOPacket)
			if err != nil {
				c.onError(err)
				return
			}
		}
	}
}

func (c *serverConn) onFinishEIOPacket(header *parser.PacketHeader, eventName string, decode parser.Decode) {
	if header.Namespace == "" {
		header.Namespace = "/"
	}

	if header.Type == parser.PacketTypeConnect {
		c.connect(header, decode)
	} else {
		socket, ok := c.sockets.Get(header.Namespace)
		if !ok {
			return
		}
		socket.onPacket(header, eventName, decode)
	}
}

func (c *serverConn) connect(header *parser.PacketHeader, decode parser.Decode) {
	var auth json.RawMessage

	at := reflect.TypeOf(&auth)
	values, err := decode(at)
	if err != nil {
		panic(err)
	}

	if len(values) == 1 {
		rmp, ok := values[0].Interface().(*json.RawMessage)
		if ok {
			auth = *rmp
		}
	}

	nsp := c.server.Of(header.Namespace)
	socket, err := nsp.add(c, auth)
	if err != nil {
		c.connectError(err, nsp.Name())
		return
	}

	c.sockets.Set(socket)
	c.nsps.Set(nsp)

	socket.onConnect()

	f, ok := c.server.onSocketHandler.Load().(OnSocketCallback)
	if ok {
		f(socket)
	}
}

func (c *serverConn) connectError(err error, nsp string) {
	e := &connectError{
		Message: err.Error(),
	}

	header := parser.PacketHeader{
		Type:      parser.PacketTypeConnectError,
		Namespace: nsp,
	}

	buffers, err := c.parser.Encode(&header, e)
	if err != nil {
		panic(err)
	}

	c.sendBuffers(buffers...)
}

func (c *serverConn) sendBuffers(buffers ...[]byte) {
	if len(buffers) > 0 {
		packets := make([]*eioparser.Packet, len(buffers))
		buf := buffers[0]
		buffers = buffers[1:]

		var err error
		packets[0], err = eioparser.NewPacket(eioparser.PacketTypeMessage, false, buf)
		if err != nil {
			c.onError(err)
			return
		}

		for i, attachment := range buffers {
			packets[i+1], err = eioparser.NewPacket(eioparser.PacketTypeMessage, true, attachment)
			if err != nil {
				c.onError(err)
				return
			}
		}

		c.packet(packets...)
	}
}

func (c *serverConn) packet(packets ...*eioparser.Packet) {
	go c.eio.Send(packets...)
}

func (c *serverConn) onError(err error) {

}

func (c *serverConn) onClose(reason string, err error) {

}