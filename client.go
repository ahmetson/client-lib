// Package client defines client zmqSocket that can access to the client service.
package client

import (
	"fmt"
	"github.com/ahmetson/client-lib/config"
	"time"

	"github.com/ahmetson/common-lib/message"
	zmq "github.com/pebbe/zmq4"
)

const (
	minTimeout = time.Millisecond * 2
	minAttempt = uint8(1)
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
	if socket.zmqSocket == nil {
		return nil
	}
	err := socket.zmqSocket.Close()
	if err != nil {
		return fmt.Errorf("error closing zmqSocket: %w", err)
	}

	return nil
}

// NewRaw client based on the target
func NewRaw(target zmq.Type, url string) (*Socket, error) {
	if !config.IsTarget(target) {
		return nil, fmt.Errorf("target is not supported")
	}
	socketType := config.TargetToClient(target)
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
	url := client.Url()
	if len(url) == 0 {
		return nil, fmt.Errorf("url not set. context not linked")
	}

	socket, err := NewRaw(client.TargetType, url)
	if err != nil {
		return nil, fmt.Errorf("newRaw('%s', '%s'): %w", client.TargetType.String(), url, err)
	}
	socket.config = client

	return socket, nil
}

// Timeout update. If the timeout is less than minTimeout, then minTimeout is set
func (socket *Socket) Timeout(timeout time.Duration) *Socket {
	if timeout < minTimeout {
		timeout = minTimeout
	}

	socket.timeout = timeout
	return socket
}

// Attempt update. If the attempt is less than minAttempt, then minAttempt is set
func (socket *Socket) Attempt(attempt uint8) *Socket {
	if attempt < minAttempt {
		attempt = minAttempt
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

// rawSubmit sends the message; it doesn't wait for a reply to see was it successfully sent.
func (socket *Socket) rawSubmit(raw string) error {
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

// RawSubmit sends the message to the destination, without waiting for the reply.
// If the socket has to wait for a reply, otherwise its blocking,
// then the RawSubmit will receive the message, but omit it.
func (socket *Socket) RawSubmit(raw string) error {
	err := socket.rawSubmit(raw)
	if err != nil {
		return fmt.Errorf("socket.rawSubmit: %w", err)
	}

	return nil
}

func (socket *Socket) RawRequest(raw string) ([]string, error) {
	err := socket.rawSubmit(raw)
	if err != nil {
		return nil, fmt.Errorf("socket.rawSubmit: %w", err)
	}
	socket.pollIn(true)

	attempt := socket.attempt

	for {
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
