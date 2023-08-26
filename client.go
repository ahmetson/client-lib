// Package client defines client zmqSocket that can access to the client service.
package client

import (
	"fmt"
	"github.com/ahmetson/client-lib/config"
	"time"

	"github.com/ahmetson/common-lib/message"
	zmq "github.com/pebbe/zmq4"
)

// Socket is the wrapper around zeromq's zmqSocket.
// The zmqSocket is the client's zmqSocket that will try to interact with the client service.
type Socket struct {
	poller     *zmq.Poller
	zmqSocket  *zmq.Socket
	url        string
	timeout    time.Duration
	attempt    uint8
	socketType zmq.Type
	target     zmq.Type
	config     *config.Client
}

// Attempts to connect to the endpoint.
// The difference from zmqSocket.reconnect() is that it will not authenticate if security is enabled.
func (socket *Socket) reconnect() error {
	if socket.zmqSocket != nil {
		if err := socket.Close(); err != nil {
			return fmt.Errorf("failed to close zmqSocket in zmq: %w", err)
		}
		socket.zmqSocket = nil
	}

	var err error
	socket.zmqSocket, err = zmq.NewSocket(socket.socketType)
	if err != nil {
		return fmt.Errorf("zmq.NewSocket('%s'): %w", socket.socketType.String(), err)
	}

	if err := socket.zmqSocket.SetLinger(0); err != nil {
		return fmt.Errorf("zmqSocket.SetLinger(0): %w", err)
	}

	if err := socket.zmqSocket.Connect(socket.url); err != nil {
		return fmt.Errorf("zmqSocket.Connect('%s'): %w", socket.url, err)
	}

	socket.poller = zmq.NewPoller()

	return nil
}

func (socket *Socket) pollIn(update bool) {
	if update {
		_, _ = socket.poller.UpdateBySocket(socket.zmqSocket, zmq.POLLIN)
	} else {
		_ = socket.poller.Add(socket.zmqSocket, zmq.POLLIN)
	}
}

func (socket *Socket) pollOut() {
	_ = socket.poller.Add(socket.zmqSocket, zmq.POLLOUT)
}

// Close the zmqSocket free the port and resources.
func (socket *Socket) Close() error {
	err := socket.zmqSocket.Close()
	if err != nil {
		return fmt.Errorf("error closing zmqSocket: %w", err)
	}

	return nil
}

// NewRaw client based on the target
func NewRaw(target zmq.Type, url string) (*Socket, error) {
	if !IsTarget(target) {
		return nil, fmt.Errorf("target is not supported")
	}
	socketType := SocketType(target)
	socket := &Socket{
		zmqSocket:  nil,
		timeout:    time.Second * 10,
		attempt:    5,
		target:     target,
		socketType: socketType,
		url:        url,
		config:     nil,
	}

	return socket, nil
}

// New client based on the configuration
func New(client *config.Client) (*Socket, error) {
	if !IsTarget(client.TargetType) {
		return nil, fmt.Errorf("target is not supported")
	}
	url := client.Url()
	if len(url) == 0 {
		return nil, fmt.Errorf("url not set. context not linked")
	}

	socketType := SocketType(client.TargetType)
	socket := &Socket{
		zmqSocket:  nil,
		timeout:    time.Second * 10,
		attempt:    5,
		target:     client.TargetType,
		socketType: socketType,
		url:        url,
		config:     client,
	}

	return socket, nil
}

func IsTarget(target zmq.Type) bool {
	return target == zmq.REP || target == zmq.ROUTER || target == zmq.PUB || target == zmq.PUSH || target == zmq.PULL
}

// SocketType gets the ZMQ analog of the handler type for the clients
func SocketType(target zmq.Type) zmq.Type {
	switch target {
	case zmq.PUB:
		return zmq.SUB
	case zmq.PUSH:
		return zmq.PULL
	case zmq.PULL:
		return zmq.PUSH
	default:
		// For zmq.REP and zmq.ROUTER
		return zmq.REQ
	}
}

func (socket *Socket) Timeout(timeout time.Duration) *Socket {
	if timeout < time.Millisecond*2 {
		timeout = time.Millisecond
	}

	socket.timeout = timeout
	return socket
}

func (socket *Socket) Attempt(attempt uint8) *Socket {
	if attempt < 1 {
		attempt = 1
	}

	socket.attempt = attempt
	return socket
}

// Request sends the request message to the zmqSocket.
// Returns the message.Reply.Parameters in case of success.
//
// Error is returned in other cases.
//
// If the client service returned a failure message, it's converted into an error.
//
// The zmqSocket type should be REQ or PUSH.
func (socket *Socket) Request(req *message.Request) (*message.Reply, error) {
	reqStr, err := req.String()
	if err != nil {
		return nil, fmt.Errorf("request.String: %w", err)
	}

	rawReply, err := socket.RawRequest(reqStr)
	if err != nil {
		return nil, fmt.Errorf("socket.RawRequest: %w", err)
	}

	reply, err := message.ParseReply(rawReply)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the command '%s': %w", req.Command, err)
	}

	// client service will add its own stack.
	req.SyncTrace(&reply)

	return &reply, nil
}

func (socket *Socket) Submit(req *message.Request) error {
	reqStr, err := req.String()
	if err != nil {
		return fmt.Errorf("request.String: %w", err)
	}

	err = socket.RawSubmit(reqStr)
	if err != nil {
		return fmt.Errorf("socket.RawRequest: %w", err)
	}

	return nil
}

func (socket *Socket) RawSubmit(raw string) error {
	err := socket.reconnect()
	if err != nil {
		return fmt.Errorf("initial  socket.reconnect: %w", err)
	}

	socket.pollOut()

	attempt := socket.attempt

	messages := []string{raw}
	if socket.socketType == zmq.DEALER {
		messages = []string{"", raw}
	}

	for {
		// Poll zmqSocket for a reply, with timeout
		sockets, err := socket.poller.Poll(socket.timeout)
		if err != nil {
			return fmt.Errorf("poll error: %w", err)
		}

		if len(sockets) > 0 {
			//  We send a request, then we work to get a reply
			if _, err := socket.zmqSocket.SendMessageDontwait(messages); err != nil {
				return fmt.Errorf("zmqSocket.SendMessage: %w", err)
			}

			return nil
		}

		err = socket.reconnect()
		if err != nil {
			return fmt.Errorf("zmqSocket.reconnect: %w", err)
		}

		socket.pollOut()

		attempt--
		if attempt == 0 {
			return fmt.Errorf("timeout")
		}
	}
}

func (socket *Socket) RawRequest(raw string) ([]string, error) {
	err := socket.RawSubmit(raw)
	if err != nil {
		return nil, fmt.Errorf("socket.RawSubmit: %w", err)
	}
	socket.pollIn(true)

	attempt := socket.attempt

	messages := []string{raw}
	if socket.socketType == zmq.DEALER {
		messages = []string{"", raw}
	}

	for {
		//  We send a request, then we work to get a reply
		if _, err := socket.zmqSocket.SendMessage(messages); err != nil {
			return nil, fmt.Errorf("zmqSocket.SendMessage: %w", err)
		}

		// Poll zmqSocket for a reply, with timeout
		sockets, err := socket.poller.Poll(socket.timeout)
		if err != nil {
			return nil, fmt.Errorf("poll error: %w", err)
		}

		if len(sockets) > 0 {
			// Wait for a reply.
			r, err := socket.zmqSocket.RecvMessage(0)
			if err != nil {
				return nil, fmt.Errorf("zmqSocket.RecvMessage: %w", err)
			}

			return r, nil
		}
		err = socket.reconnect()
		if err != nil {
			return nil, fmt.Errorf("zmqSocket.reconnect: %w", err)
		}

		socket.pollIn(false)

		attempt--
		if attempt == 0 {
			return nil, fmt.Errorf("timeout")
		}
	}
}
