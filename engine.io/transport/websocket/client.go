package websocket

import (
	"net/url"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/tomruk/socket.io-go/engine.io/parser"
	"github.com/tomruk/socket.io-go/engine.io/transport"
)

type ClientTransport struct {
	sid string

	protocolVersion int
	url             *url.URL
	requestHeader   *transport.RequestHeader

	dialer  *websocket.Dialer
	conn    *websocket.Conn
	writeMu sync.Mutex

	callbacks *transport.Callbacks

	once sync.Once
}

func NewClientTransport(callbacks *transport.Callbacks, sid string, protocolVersion int, url url.URL, requestHeader *transport.RequestHeader, dialer *websocket.Dialer) *ClientTransport {
	return &ClientTransport{
		sid: sid,

		protocolVersion: protocolVersion,
		url:             &url,
		requestHeader:   requestHeader,

		callbacks: callbacks,

		dialer: dialer,
	}
}

func (t *ClientTransport) Name() string {
	return "websocket"
}

func (t *ClientTransport) Callbacks() *transport.Callbacks {
	return t.callbacks
}

func (t *ClientTransport) Handshake() (hr *parser.HandshakeResponse, err error) {
	q := t.url.Query()
	q.Set("transport", "websocket")
	q.Set("EIO", strconv.Itoa(t.protocolVersion))

	if t.sid != "" {
		q.Set("sid", t.sid)
	}

	t.url.RawQuery = q.Encode()

	switch t.url.Scheme {
	case "https":
		t.url.Scheme = "wss"
	case "http":
		t.url.Scheme = "ws"
	}

	t.conn, _, err = t.dialer.Dial(t.url.String(), t.requestHeader.Header())
	if err != nil {
		return
	}

	// If sid is set this means that we have already connected and
	// we're using this transport for upgrade purposes.

	// If sid is not set, we should receive the OPEN packet and return the values decoded from it.
	if t.sid == "" {
		p, err := t.nextPacket()
		if err != nil {
			return nil, err
		}

		hr, err = parser.ParseHandshakeResponse(p)
		if err != nil {
			return nil, err
		}

		t.sid = hr.SID
	}

	return
}

func (t *ClientTransport) Run() {
	for {
		p, err := t.nextPacket()
		if err != nil {
			t.close(err)
			return
		}

		t.callbacks.OnPacket(p)
	}
}

func (t *ClientTransport) nextPacket() (*parser.Packet, error) {
	mt, r, err := t.conn.NextReader()
	if err != nil {
		return nil, err
	}
	return parser.Decode(r, mt == websocket.BinaryMessage)
}

func (t *ClientTransport) Send(packets ...*parser.Packet) {
	// WriteMessage must not be called concurrently.
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	for _, p := range packets {
		var mt int
		if p.IsBinary {
			mt = websocket.BinaryMessage
		} else {
			mt = websocket.TextMessage
		}

		w, err := t.conn.NextWriter(mt)
		if err != nil {
			t.close(err)
			break
		}

		err = p.Encode(w, true)
		if err != nil {
			t.close(err)
			break
		}
	}
}

func (t *ClientTransport) Discard() {
	t.once.Do(func() {
		if t.conn != nil {
			t.conn.Close()
		}
	})
}

func (t *ClientTransport) close(err error) {
	t.once.Do(func() {
		if websocket.IsCloseError(err, expectedCloseCodes...) {
			err = nil
		}

		defer t.callbacks.OnClose(t.Name(), err)

		if t.conn != nil {
			t.conn.Close()
		}
	})
}

func (t *ClientTransport) Close() {
	t.close(nil)
}
