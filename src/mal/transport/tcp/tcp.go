/**
 * MIT License
 *
 * Copyright (c) 2017 CNES
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */
package tcp

import (
	. "mal"
	"mal/debug"
	"net"
	"net/url"
	"strconv"
)

const (
	NETWORK_PROPERTY string = "network"

	VARIABLE_LENGTH_OFFSET uint32 = 19
	FIXED_HEADER_LENGTH    uint32 = 23
)

var (
	logger debug.Logger = debug.GetLogger("mal.transport.tcp")
)

type TCPTransport struct {
	uri    URI
	ctx    TransportCallback
	params map[string][]string

	version byte

	network string
	address string
	port    uint16

	running bool

	ch   chan *Message
	ends chan bool

	listen net.Listener
	conns  map[string]net.Conn

	sourceFlag           bool
	destinatioFlag       bool
	priorityFlag         bool
	timestampFlag        bool
	networkZoneFlag      bool
	sessionNameFlag      bool
	domainFlag           bool
	authenticationIdFlag bool

	flags byte

	dfltPriority         UInteger
	dfltNetworkZone      Identifier
	dfltSessionName      Identifier
	dfltAuthenticationId Blob
	dfltDomain           IdentifierList
}

func (transport *TCPTransport) init() error {
	transport.running = false

	// TODO (AF): Configure flags
	transport.flags = 0
	// Note (AF): Should be always true
	transport.sourceFlag = true
	if transport.sourceFlag {
		transport.flags |= (1 << 7)
	}
	// Note (AF): Should be always true
	transport.destinatioFlag = true
	if transport.destinatioFlag {
		transport.flags |= (1 << 6)
	}
	transport.priorityFlag = true
	if transport.priorityFlag {
		transport.flags |= (1 << 5)
	}
	transport.timestampFlag = true
	if transport.timestampFlag {
		transport.flags |= (1 << 4)
	}
	transport.networkZoneFlag = true
	if transport.networkZoneFlag {
		transport.flags |= (1 << 3)
	}
	transport.sessionNameFlag = true
	if transport.sessionNameFlag {
		transport.flags |= (1 << 2)
	}
	transport.domainFlag = true
	if transport.domainFlag {
		transport.flags |= (1 << 1)
	}
	transport.authenticationIdFlag = true
	if transport.authenticationIdFlag {
		transport.flags |= 1
	}

	// Get protocol: tcp, tcp4 or tcp6.
	if p := transport.params[NETWORK_PROPERTY]; p != nil {
		transport.network = p[0]
	} else {
		transport.network = "tcp"
	}

	transport.conns = make(map[string]net.Conn)
	// TODO (AF): Fix length of channel
	transport.ch = make(chan *Message, 10)
	transport.ends = make(chan bool)

	return nil
}

func (transport *TCPTransport) start() error {
	// If the host in the address parameter is empty or a literal unspecified IP address,
	// Listen listens on all available unicast and anycast IP addresses of the local system.
	// To only use IPv4, use "tcp4" a network parameter.
	listen, err := net.Listen(transport.network, ":"+strconv.Itoa(int(transport.port)))
	if err != nil {
		// TODO (AF): Log an error
		return err
	}

	transport.running = true

	transport.listen = listen
	go transport.handleConn(listen)
	go transport.handleOut()

	return nil
}

func (transport *TCPTransport) handleConn(listen net.Listener) {
	for {
		cnx, err := listen.Accept()
		if err != nil {
			// TODO (AF): handle error
			break
		}
		logger.Infof("Accept connexion from %s", cnx.RemoteAddr())
		// TODO (AF): Registers new connection
		// transport.conns[uri] = cnx
		go transport.handleIn(cnx)
	}
	logger.Infof("HandleConn exited")
}

func (transport *TCPTransport) handleIn(cnx net.Conn) {
	for transport.running {
		logger.Debugf("HandleIn wait for message: %s", cnx.RemoteAddr())
		msg, err := transport.readMessage(cnx)

		if err != nil {
			// TODO (AF): handle error
			continue
		}
		logger.Debugf("Receives message: %s", msg)
		if msg != nil {
			transport.ctx.Receive(msg)
		}
	}
	logger.Infof("HandleIn exited: %s", cnx.RemoteAddr())
}

func (transport *TCPTransport) readMessage(cnx net.Conn) (*Message, error) {
	// TODO (AF): May be this array should be reused
	var buf []byte = make([]byte, FIXED_HEADER_LENGTH)

	// Reads the fixed part of MAL message header
	for offset := 0; offset < int(FIXED_HEADER_LENGTH); {
		nb, err := cnx.Read(buf[offset:])
		if err != nil {
			// TODO (AF): handle error
			return nil, err
		}
		offset += nb
	}

	// Get the variable length of message
	length := FIXED_HEADER_LENGTH +
		uint32(buf[VARIABLE_LENGTH_OFFSET+3]) | uint32(buf[VARIABLE_LENGTH_OFFSET+2])<<8 |
		uint32(buf[VARIABLE_LENGTH_OFFSET+1])<<16 | uint32(buf[VARIABLE_LENGTH_OFFSET])<<24
	logger.Debugf("Reads message header, length: %d", length)

	// Allocate a new buffer and copy the fixed part of MAL message header
	var newbuf []byte = make([]byte, length)
	copy(newbuf, buf)

	// Reads fully the message
	for offset := int(FIXED_HEADER_LENGTH); offset < len(newbuf); {
		nb, err := cnx.Read(newbuf[offset:])
		if err != nil {
			// TODO (AF): handle error
			return nil, err
		}
		offset += nb
		logger.Debugf("Reads: %d", offset)
	}

	// Decodes the message
	msg, err := transport.decode(newbuf, cnx.RemoteAddr().String())
	// TODO (AF): Optimized URI mapping
	//	if msg.UriTo == nil {
	//		var urito URI = transport.uri
	//		msg.UriTo = &urito
	//	}
	//	if msg.UriFrom == nil {
	//		var urifrom
	//		msg.UriFrom = &URI("maltcp://" + cnx.RemoteAddr().String())
	//	}
	if err != nil {
		// TODO (AF): handle error
		logger.Errorf("##### Errors receiving message: %s", err)
		return nil, err
	}
	logger.Debugf("##### Receives: %s from %s to %s", msg, *msg.UriFrom, *msg.UriTo)

	return msg, nil
}

func (transport *TCPTransport) handleOut() {
	for {
		logger.Debugf("handleOut: wait message")
		msg, more := <-transport.ch
		if more {
			logger.Debugf("handleOut: get Message%+v", *msg)
			u, err := url.Parse(string(*msg.UriTo))
			if err != nil {
				logger.Errorf("Cannot route message, urito=%s", *msg.UriTo)
				continue
			}
			// TODO (AF):
			//		urito := url.URL{Scheme: u.Scheme, Host: u.Host}
			urito := u.Host

			cnx, ok := transport.conns[urito]
			if !ok {
				logger.Debugf("Creates connection to %s", urito)
				cnx, err = net.Dial("tcp", urito)
				if err != nil {
					// TODO (AF): handles error
					logger.Errorf("HandleOut: %s", err)
					continue
				}
				transport.conns[urito] = cnx
			}
			logger.Debugf("%s, %s", *msg.UriFrom, *msg.UriTo)
			err = transport.writeMessage(cnx, msg)
			if err != nil {
				// TODO (AF): handle error
				logger.Debugf("HandleOut: %s", err)
			}
		} else {
			logger.Infof("MALTCP Context ends: %+v", msg)
			transport.ends <- true
		}
	}
	logger.Debugf("HandleOut exited")
}

func write32(value uint32, buf []byte) {
	buf[0] = byte(value >> 24)
	buf[1] = byte(value >> 16)
	buf[2] = byte(value >> 8)
	buf[3] = byte(value >> 0)
}

func (transport *TCPTransport) writeMessage(cnx net.Conn, msg *Message) error {
	buf, err := transport.encode(msg)
	if err != nil {
		// TODO (AF): Logging
		return err
	}
	logger.Debugf("Writes message: %d", len(buf))
	write32(uint32(len(buf))-FIXED_HEADER_LENGTH, buf[VARIABLE_LENGTH_OFFSET:VARIABLE_LENGTH_OFFSET+4])
	logger.Debugf("Message transmitted: ", buf)
	_, err = cnx.Write(buf)
	if err != nil {
		// TODO (AF): Logging
		return err
	}
	return nil
}

func (transport *TCPTransport) Transmit(msg *Message) error {
	logger.Debugf("Transmit: %+v", *msg)
	transport.ch <- msg
	logger.Debugf("Transmited")
	return nil
}

func (transport *TCPTransport) TransmitMultiple(msgs ...*Message) error {
	for _, msg := range msgs {
		err := transport.Transmit(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (transport *TCPTransport) Close() error {
	transport.running = false
	close(transport.ch)
	transport.listen.Close()
	for _, cnx := range transport.conns {
		cnx.Close()
	}
	// TODO (AF):
	return nil
}
