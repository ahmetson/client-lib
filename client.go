// Package client defines client zmqSocket that can access to the client service.
package client

import (
	"fmt"
	"time"

	"github.com/ahmetson/common-lib/data_type/key_value"
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
	socket.poller.Add(socket.zmqSocket, zmq.POLLIN)

	return nil
}

// Close the zmqSocket free the port and resources.
func (socket *Socket) Close() error {
	err := socket.zmqSocket.Close()
	if err != nil {
		return fmt.Errorf("error closing zmqSocket: %w", err)
	}

	return nil
}

// New client based on the target
func New(target zmq.Type, url string) (*Socket, error) {
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
func (socket *Socket) Request(req *message.Request) (key_value.KeyValue, error) {
	err := socket.reconnect()
	if err != nil {
		return nil, fmt.Errorf("zmqSocket connection: %w", err)
	}

	requestString, err := req.String()
	if err != nil {
		return nil, fmt.Errorf("request.String: %w", err)
	}

	attempt := socket.attempt

	// we attempt requests for an infinite amount of time.
	for {
		//  We send a request, then we work to get a reply
		if _, err := socket.zmqSocket.SendMessage(requestString); err != nil {
			return nil, fmt.Errorf("failed to send the command '%s'. zmqSocket error: %w", req.Command, err)
		}

		//  Poll zmqSocket for a reply, with timeout
		sockets, err := socket.poller.Poll(socket.timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to to send the command '%s'. poll error: %w", req.Command, err)
		}

		//  Here we process a server reply and exit our loop if the
		//  reply is valid.
		// If we didn't have a reply, we close the client
		//  zmqSocket and resend the request.
		// We try a number of times
		//  before finally abandoning:

		if len(sockets) > 0 {
			// Wait for a reply.
			r, err := socket.zmqSocket.RecvMessage(0)
			if err != nil {
				return nil, fmt.Errorf("failed to receive the command '%s' message. zmqSocket error: %w", req.Command, err)
			}

			reply, err := message.ParseReply(r)
			if err != nil {
				return nil, fmt.Errorf("failed to parse the command '%s': %w", req.Command, err)
			}

			// client service will add its own stack.
			req.SyncTrace(&reply)

			if !reply.IsOK() {
				return nil, fmt.Errorf("the command '%s' replied with a failure: %s", req.Command, reply.Message)
			}

			return reply.Parameters, nil
		} else {
			if attempt == 0 {
				return nil, fmt.Errorf("timeout")
			}
			attempt--

			err := socket.reconnect()
			if err != nil {
				return nil, fmt.Errorf("zmqSocket.inproc_reconnect: %w", err)
			}
		}
	}
}

func (socket *Socket) RequestRawMessage(requestString string) ([]string, error) {
	err := socket.reconnect()
	if err != nil {
		return nil, fmt.Errorf("zmqSocket connection: %w", err)
	}

	attempt := socket.attempt

	// we attempt requests for an infinite amount of time.
	for {
		//  We send a request, then we work to get a reply
		if _, err := socket.zmqSocket.SendMessage(requestString); err != nil {
			return nil, fmt.Errorf("failed to send the message. zmqSocket error: %w", err)
		}

		// Poll zmqSocket for a reply, with timeout
		sockets, err := socket.poller.Poll(socket.timeout)
		if err != nil {
			return nil, fmt.Errorf("poll error: %w", err)
		}

		// Here we process a server reply and exit our loop if the
		//  reply is valid.
		// If we didn't have a reply, we close the client
		//  zmqSocket and resend the request.
		// We try a number of times
		//  before finally abandoning:

		if len(sockets) > 0 {
			// Wait for a reply.
			r, err := socket.zmqSocket.RecvMessage(0)
			if err != nil {
				return nil, fmt.Errorf("failed to message. zmqSocket error: %w", err)
			}

			return r, nil
		} else {
			if attempt == 0 {
				return nil, fmt.Errorf("timeout")
			}
			attempt--

			err := socket.reconnect()
			if err != nil {
				return nil, fmt.Errorf("zmqSocket.inproc_reconnect: %w", err)
			}
		}
	}
}

// Url creates url of the server for the client to connect
//
// If the port is 0, then the client will be inproc, not as tcp
func Url(name string, port uint64) string {
	if port == 0 {
		return fmt.Sprintf("inproc://%s", name)
	}
	url := fmt.Sprintf("tcp://localhost:%d", port)
	return url
}
