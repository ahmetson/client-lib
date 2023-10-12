// Package client defines client zmqSocket that can access to the client service.
package client

import (
	"fmt"
	"github.com/ahmetson/client-lib/config"
	"github.com/ahmetson/datatype-lib/data_type"
	"github.com/ahmetson/datatype-lib/message"
	"os"
	"time"

	zmq "github.com/pebbe/zmq4"
)

const (
	minTimeout     = time.Millisecond * 2
	DefaultTimeout = time.Second * 100

	minAttempt     = uint8(1)
	DefaultAttempt = uint8(5)
)

type Message struct {
	Submit chan []string
	Raw    string
}

// A Socket that connects to the handler.
type Socket struct {
	consumerId uint64 // consume internal id assigned by zeromq
	poller     *zmq.Poller
	schedulers *zmq.Reactor // client keeps a zmqSocket for initialing a message transfer, and a queue consumer.
	zmqSocket  *zmq.Socket
	url        string
	timeout    time.Duration
	attempt    uint8
	socketType zmq.Type
	target     zmq.Type
	config     *config.Client
	queue      *data_type.Queue
	sent       uint64
	messageOps *message.Operations // client translates the message before and after transmitting using message operations.
}

// NewRaw returns a new client that's connected to the given url.
// For it to work, the url must be provided with the protocol that is supported by zeromq.
// Example of valid urls:
//
//   - tcp://localhost:6000
//
//   - inproc://internal_thread
//
// Invalid url:
//
//   - http://example.com/ 	-- HTTP protocol is not supported
//
//   - ./socket.pid 			-- File descriptors are not supported
func NewRaw(target zmq.Type, url string) (*Socket, error) {
	if !config.IsTarget(target) {
		return nil, fmt.Errorf("target is not supported")
	}

	socketType := config.TargetToClient(target)
	socket := &Socket{
		zmqSocket:  nil,
		timeout:    DefaultTimeout,
		attempt:    DefaultAttempt,
		target:     target,
		socketType: socketType,
		url:        url,
		config:     nil,
		queue:      data_type.NewQueue(),
		schedulers: zmq.NewReactor(),
		consumerId: 0,
		sent:       1,
		messageOps: message.DefaultMessage(),
	}

	// we can remove the following lines
	socket.consumerId = socket.schedulers.AddChannelTime(time.Tick(time.Microsecond), 0,
		func(_ interface{}) error { return socket.handleConsume() })

	go func() {
		err := socket.schedulers.Run(time.Microsecond * 2)
		if err != nil && err.Error() != "No sockets to poll, no channels to read" {
			_, _ = fmt.Fprintf(os.Stderr, "reactor exited with an error: %v\n", err)
		}
		socket.schedulers = nil
	}()

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

func (socket *Socket) SetMessageOperations(messageOps *message.Operations) {
	socket.messageOps = messageOps
}

// handleConsume runs in a loop to read the queue.
// For the given queue, it will send the message to the handler.
func (socket *Socket) handleConsume() error {
	if socket.queue.IsEmpty() {
		return nil
	}

	msg := socket.queue.Pop().(*Message)

	if msg.Submit == nil {
		err := socket.rawSubmitByTimeout(msg.Raw)
		if err != nil {
			return fmt.Errorf("socket.rawSubmitByTimeout: %w", err)
		}
		return nil
	}

	reply, err := socket.rawRequestByTimeout(msg.Raw)

	msg.Submit <- reply

	if err != nil {
		return fmt.Errorf("socket.RawSocket: %w", err)
	}

	return nil
}

// Attempts to connect to the endpoint.
// The difference from zmqSocket.reconnect() is that it will not authenticate if security is enabled.
func (socket *Socket) reconnect() (err error) {
	if socket.zmqSocket != nil {
		if err := socket.zmqSocket.Close(); err != nil {
			return fmt.Errorf("failed to close zmqSocket in zmq: %w", err)
		}
	}

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

func (socket *Socket) updateToPollIn() {
	_, _ = socket.poller.UpdateBySocket(socket.zmqSocket, zmq.POLLIN)
}

func (socket *Socket) pollOut() {
	_ = socket.poller.Add(socket.zmqSocket, zmq.POLLOUT|zmq.POLLIN)
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

	if socket.schedulers != nil {
		socket.schedulers.RemoveChannel(socket.consumerId)
	}

	return nil
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

func (socket *Socket) RawRequest(raw string) ([]string, error) {
	if socket.queue.IsFull() {
		return nil, fmt.Errorf("queue is full, try again later")
	}

	// todo, a message channel must return the error as well
	// todo, rename Message to Send and Message.Submit a type to Reply.
	msg := &Message{
		Submit: make(chan []string),
		Raw:    raw,
	}
	socket.queue.Push(msg)

	reply := <-msg.Submit

	return reply, nil
}

func (socket *Socket) rawRequestByTimeout(raw string) ([]string, error) {
	// Since we decrement before an attempt, it will be 0
	// If we had 1 attempt.
	attempt := socket.attempt + 1

	for {
		attempt--
		if attempt == 0 {
			return nil, fmt.Errorf("timeout")
		}

		timeout, err := socket.rawSubmit(raw)
		if err != nil {
			return nil, fmt.Errorf("socket.rawSubmit: %w", err)
		}

		if timeout {
			continue
		}

		socket.updateToPollIn()

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
	}
}

// RawSubmit sends the message to the destination, without waiting for the reply.
// If the socket has to wait for a reply, otherwise its blocking,
// then the RawSubmit will receive the message, but omit it.
func (socket *Socket) RawSubmit(raw string) error {
	if socket.queue.IsFull() {
		return fmt.Errorf("queue is full, try again later")
	}

	msg := &Message{
		Submit: nil,
		Raw:    raw,
	}
	socket.queue.Push(msg)

	return nil
}

func (socket *Socket) rawSubmitByTimeout(raw string) error {
	attempt := socket.attempt

	for {
		timeout, err := socket.rawSubmit(raw)
		if err != nil {
			return fmt.Errorf("socket.rawSubmit: %w", err)
		}

		if !timeout {
			break
		}

		attempt--
		if attempt == 0 {
			return fmt.Errorf("submit timeout")
		}
	}
	return nil
}

// rawSubmit sends the message; it doesn't wait for a reply to see was it successfully sent.
//
// returns boolean for timeout.
func (socket *Socket) rawSubmit(raw string) (bool, error) {
	// no need to reconnect every time.
	err := socket.reconnect()
	if err != nil {
		return false, fmt.Errorf("initial  socket.reconnect: %w", err)
	}

	socket.pollOut()

	messages := []string{raw}
	if socket.socketType == zmq.DEALER {
		messages = []string{"", raw}
	} else if socket.socketType == zmq.PAIR {
		messages = []string{fmt.Sprintf("%d", socket.sent), "", raw}
		socket.sent++
	} else if socket.socketType == zmq.REQ && socket.target == zmq.ROUTER {
		messages = []string{"", raw}
	}

	// Poll zmqSocket for a reply, with timeout
	sockets, err := socket.poller.Poll(socket.timeout)
	if err != nil {
		return false, fmt.Errorf("poll error: %w", err)
	}

	if len(sockets) > 0 {
		//  We send a request, then we work to get a reply
		if _, err := socket.zmqSocket.SendMessage(messages); err != nil {
			return false, fmt.Errorf("zmqSocket.SendMessage: %w", err)
		}

		// message received without a timeout
		return false, nil
	}

	return true, nil
}

//
// The client that works with the message.RequestInterface and message.ReplyInterface
//

func (socket *Socket) Submit(req message.RequestInterface) error {
	reqStr, err := req.ZmqEnvelope()
	if err != nil {
		return fmt.Errorf("request.String: %w", err)
	}

	err = socket.RawSubmit(message.JoinMessages(reqStr))
	if err != nil {
		return fmt.Errorf("socket.RawRequest: %w", err)
	}

	return nil
}

// Request sends the request message to the zmqSocket.
// Returns the message.Reply.Parameters in case of success.
//
// Error is returned in other cases.
//
// If the client service returned a failure message, it's converted into an error.
//
// The zmqSocket type should be REQ or PUSH.
func (socket *Socket) Request(req message.RequestInterface) (message.ReplyInterface, error) {
	reqStr, err := req.ZmqEnvelope()
	if err != nil {
		return nil, fmt.Errorf("request.String: %w", err)
	}

	rawReply, err := socket.RawRequest(message.JoinMessages(reqStr))
	if err != nil {
		return nil, fmt.Errorf("socket.RawRequest: %w", err)
	}

	reply, err := socket.messageOps.NewReply(rawReply)
	if err != nil {
		return nil, fmt.Errorf("messageOps.NewReply('%v'): %w", rawReply, err)
	}

	// client service will add its own stack.
	req.SyncTrace(reply)

	return reply, nil
}
