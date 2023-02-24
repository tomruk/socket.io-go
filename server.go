package sio

import (
	"net/http"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	eio "github.com/tomruk/socket.io-go/engine.io"
	"github.com/tomruk/socket.io-go/parser"
	jsonparser "github.com/tomruk/socket.io-go/parser/json"
	"github.com/tomruk/socket.io-go/parser/json/serializer/stdjson"
)

const DefaultConnectTimeout = time.Second * 45

type ServerConfig struct {
	ParserCreator  parser.Creator
	AdapterCreator AdapterCreator

	EIO eio.ServerConfig

	// Duration to wait before a client without namespace is closed.
	//
	// Default: 45 seconds
	ConnectTimeout time.Duration

	// In order for a client to make a connection to a namespace,
	// the namespace must be created on server via `Server.of`.
	//
	// This option permits the client to create the namespace if it is not already created on server.
	// If this option is disabled, only namespaces created on the server can be connected.
	//
	// Default: false
	AcceptAnyNamespace bool
}

type Server struct {
	parserCreator  parser.Creator
	adapterCreator AdapterCreator

	eio        *eio.Server
	namespaces *namespaceStore

	connectTimeout     time.Duration
	acceptAnyNamespace bool
}

func NewServer(config *ServerConfig) *Server {
	if config == nil {
		config = new(ServerConfig)
	}

	server := &Server{
		parserCreator:      config.ParserCreator,
		adapterCreator:     config.AdapterCreator,
		namespaces:         newNamespaceStore(),
		acceptAnyNamespace: config.AcceptAnyNamespace,
	}

	server.eio = eio.NewServer(server.onEIOSocket, &config.EIO)

	if server.parserCreator == nil {
		json := stdjson.New()
		server.parserCreator = jsonparser.NewCreator(0, json)
	}

	if server.adapterCreator == nil {
		server.adapterCreator = newInMemoryAdapter
	}

	if config.ConnectTimeout != 0 {
		server.connectTimeout = config.ConnectTimeout
	} else {
		server.connectTimeout = DefaultConnectTimeout
	}

	return server
}

func (s *Server) onEIOSocket(eioSocket eio.ServerSocket) *eio.Callbacks {
	_, callbacks := newServerConn(s, eioSocket, s.parserCreator)
	return callbacks
}

func (s *Server) Of(namespace string) *Namespace {
	if len(namespace) != 0 && namespace[0] != '/' {
		namespace = "/" + namespace
	}
	return s.namespaces.GetOrCreate(namespace, s, s.adapterCreator, s.parserCreator)
}

// Alias of: s.Of("/").Use(...)
func (s *Server) Use(f MiddlewareFunction) {
	s.Of("/").Use(f)
}

// Alias of: s.Of("/").On(...)
func (s *Server) On(eventName string, handler interface{}) {
	s.Of("/").On(eventName, handler)
}

// Alias of: s.Of("/").Once(...)
func (s *Server) Once(eventName string, handler interface{}) {
	s.Of("/").Once(eventName, handler)
}

// Alias of: s.Of("/").Off(...)
func (s *Server) Off(eventName string, handler interface{}) {
	s.Of("/").Off(eventName, handler)
}

// Alias of: s.Of("/").OffAll(...)
func (s *Server) OffAll() {
	s.Of("/").OffAll()
}

func (s *Server) Emit(evetName string, v ...interface{}) {
	s.Of("/").Emit(evetName, v...)
}

// Alias of: s.Of("/").To(...)
// Sets a modifier for a subsequent event emission that the event
// will only be broadcast to clients that have joined the given room.
//
// To emit to multiple rooms, you can call `To` several times.
func (s *Server) To(room ...string) *broadcastOperator {
	return s.Of("/").To(room...)
}

// Alias of: s.Of("/").In(...)
func (s *Server) In(room ...string) *broadcastOperator {
	return s.Of("/").In(room...)
}

// Alias of: s.Of("/").To(...)
//
// Sets a modifier for a subsequent event emission that the event
// will only be broadcast to clients that have not joined the given rooms.
func (s *Server) Except(room ...string) *broadcastOperator {
	return s.Of("/").Except(room...)
}

// Alias of: s.Of("/").Compress(...)
//
// Compression flag is unused at the moment, thus setting this will have no effect on compression.
func (s *Server) Compress(compress bool) *broadcastOperator {
	return s.Of("/").Compress(compress)
}

// Alias of: s.Of("/").Local(...)
//
// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node (when scaling to multiple nodes).
//
// See: https://socket.io/docs/v4/using-multiple-nodes
func (s *Server) Local() *broadcastOperator {
	return s.Of("/").Local()
}

// Alias of: s.Of("/").Sockets(...)
//
// Gets the sockets of the namespace.
// Beware that this is local to the current node. For sockets across all nodes, use FetchSockets
func (s *Server) Sockets() []ServerSocket {
	return s.Of("/").Sockets()
}

// Alias of: s.Of("/").FetchSockets(...)
//
// Gets a list of socket IDs connected to this namespace (across all nodes if applicable).
func (s *Server) FetchSockets(room ...string) (sids mapset.Set[string]) {
	return s.Of("/").FetchSockets()
}

// Alias of: s.Of("/").SocketsJoin(...)
//
// Makes the matching socket instances leave the specified rooms.
func (s *Server) SocketsJoin(room ...string) {
	s.Of("/").SocketsJoin(room...)
}

// Alias of: s.Of("/").SocketsLeave(...)
//
// Makes the matching socket instances leave the specified rooms.
func (s *Server) SocketsLeave(room ...string) {
	s.Of("/").SocketsLeave(room...)
}

// Alias of: s.Of("/").DisconnectSockets(...)
//
// Makes the matching socket instances disconnect from the namespace.
//
// If value of close is true, closes the underlying connection. Otherwise, it just disconnects the namespace.
func (s *Server) DisconnectSockets(close bool) {
	s.Of("/").DisconnectSockets(close)
}

func (s *Server) Run() error {
	return s.eio.Run()
}

func (s *Server) PollTimeout() time.Duration {
	return s.eio.PollTimeout()
}

func (s *Server) HTTPWriteTimeout() time.Duration {
	return s.eio.HTTPWriteTimeout()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.eio.ServeHTTP(w, r)
}

func (s *Server) IsClosed() bool {
	return s.eio.IsClosed()
}

func (s *Server) Close() error {
	for _, _socket := range s.Sockets() {
		socket := _socket.(*serverSocket)
		socket.onClose("server shutting down")
	}
	return s.eio.Close()
}
