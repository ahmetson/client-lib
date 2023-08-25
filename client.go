// Package client defines client socket that can access to the client service.
package client

import (
	"fmt"
	"github.com/ahmetson/client-lib/config"
	"github.com/ahmetson/log-lib"

	"github.com/ahmetson/common-lib/data_type/key_value"
	"github.com/ahmetson/common-lib/message"
	configEngine "github.com/ahmetson/config-lib"

	// todo
	// move out dependency from security/auth
	// "github.com/ahmetson/service-lib/security/auth"
	zmq "github.com/pebbe/zmq4"
)

// ClientSocket is the wrapper around zeromq's socket.
// The socket is the client's socket that will try to interact with the client service.
type ClientSocket struct {
	// The name of client SDS service and its URL
	// its used as a clarification
	// client_credentials *auth.Credentials
	serverPublicKey string
	poller          *zmq.Poller
	socket          *zmq.Socket
	protocol        string
	logger          *log.Logger
	appConfig       *configEngine.Config
	serviceName     string
	servicePort     uint64
}

// Initiates the socket with a timeout.
// If the socket is already given, then reconnect() closes it.
// Then create a new socket.
//
// If no socket is given, then initiate a zmq.REQ socket.
func (socket *ClientSocket) reconnect() error {
	var socketCtx *zmq.Context
	var socketType zmq.Type

	if socket.socket != nil {
		ctx, err := socket.socket.Context()
		if err != nil {
			return fmt.Errorf("failed to get orchestra from zmq socket: %w", err)
		} else {
			socketCtx = ctx
		}

		socketType, err = socket.socket.GetType()
		if err != nil {
			return fmt.Errorf("failed to get socket type from zmq socket: %w", err)
		}

		err = socket.Close()
		if err != nil {
			return fmt.Errorf("failed to close socket in zmq: %w", err)
		}
		socket.socket = nil
	} else {
		return fmt.Errorf("no socket initiated: %s", "reconnect")
	}

	sock, err := socketCtx.NewSocket(socketType)
	if err != nil {
		return fmt.Errorf("failed to create %s socket: %w", socketType.String(), err)
	} else {
		socket.socket = sock
		err = socket.socket.SetLinger(0)
		if err != nil {
			return fmt.Errorf("failed to set up linger request for zmq socket: %w", err)
		}
	}

	// if socket.client_credentials != nil {
	// socket.client_credentials.SetClientAuthCurve(socket.socket, socket.server_public_key)
	// if err != nil {
	// return fmt.Errorf("auth.SetClientAuthCurve: %w", err)
	// }
	// }

	if err := socket.socket.Connect(ClientUrl(socket.serviceName, socket.servicePort)); err != nil {
		return fmt.Errorf("socket connect: %w", err)
	}

	socket.poller = zmq.NewPoller()
	socket.poller.Add(socket.socket, zmq.POLLIN)

	return nil
}

// Attempts to connect to the endpoint.
// The difference from socket.reconnect() is that it will not authenticate if security is enabled.
func (socket *ClientSocket) inprocReconnect() error {
	var socketCtx *zmq.Context
	var socketType zmq.Type

	if socket.socket != nil {
		ctx, err := socket.socket.Context()
		if err != nil {
			return fmt.Errorf("failed to get orchestra from zmq socket: %w", err)
		} else {
			socketCtx = ctx
		}

		socketType, err = socket.socket.GetType()
		if err != nil {
			return fmt.Errorf("failed to get socket type from zmq socket: %w", err)
		}

		err = socket.Close()
		if err != nil {
			return fmt.Errorf("failed to close socket in zmq: %w", err)
		}
		socket.socket = nil
	} else {
		return fmt.Errorf("failed to create zmq orchestra: %s", "inproc_reconnect")
	}

	sock, err := socketCtx.NewSocket(socketType)
	if err != nil {
		return fmt.Errorf("failed to create %s socket: %w", socketType.String(), err)
	} else {
		socket.socket = sock
		err = socket.socket.SetLinger(0)
		if err != nil {
			return fmt.Errorf("failed to set up linger request for zmq socket: %w", err)
		}
	}

	if err := socket.socket.Connect(ClientUrl(socket.serviceName, socket.servicePort)); err != nil {
		return fmt.Errorf("socket.socket.Connect: %w", err)
	}

	socket.poller = zmq.NewPoller()
	socket.poller.Add(socket.socket, zmq.POLLIN)

	return nil
}

// Close the socket free the port and resources.
func (socket *ClientSocket) Close() error {
	err := socket.socket.Close()
	if err != nil {
		return fmt.Errorf("error closing socket: %w", err)
	}

	return nil
}


// RequestRemoteService sends the request message to the socket.
// Returns the message.Reply.Parameters in case of success.
//
// Error is returned in other cases.
//
// If the client service returned a failure message, it's converted into an error.
//
// The socket type should be REQ or PUSH.
func (socket *ClientSocket) RequestRemoteService(req *message.Request) (key_value.KeyValue, error) {
	if socket.protocol == "inproc" {
		err := socket.inprocReconnect()
		if err != nil {
			return nil, fmt.Errorf("socket connection: %w", err)
		}

	} else {
		err := socket.reconnect()
		if err != nil {
			return nil, fmt.Errorf("socket connection: %w", err)
		}
	}

	requestTimeout := config.RequestTimeout(socket.appConfig)

	requestString, err := req.String()
	if err != nil {
		return nil, fmt.Errorf("request.String: %w", err)
	}

	attempt := config.Attempt(socket.appConfig)

	// we attempt requests for an infinite amount of time.
	for {
		//  We send a request, then we work to get a reply
		if _, err := socket.socket.SendMessage(requestString); err != nil {
			return nil, fmt.Errorf("failed to send the command '%s'. socket error: %w", req.Command, err)
		}

		//  Poll socket for a reply, with timeout
		sockets, err := socket.poller.Poll(requestTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to to send the command '%s'. poll error: %w", req.Command, err)
		}

		//  Here we process a server reply and exit our loop if the
		//  reply is valid.
		// If we didn't have a reply, we close the client
		//  socket and resend the request.
		// We try a number of times
		//  before finally abandoning:

		if len(sockets) > 0 {
			// Wait for a reply.
			r, err := socket.socket.RecvMessage(0)
			if err != nil {
				return nil, fmt.Errorf("failed to receive the command '%s' message. socket error: %w", req.Command, err)
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
			socket.logger.Warn("timeout", "request_command", req.Command, "attempts_left", attempt)
			if socket.protocol == "inproc" {
				err := socket.inprocReconnect()
				if err != nil {
					return nil, fmt.Errorf("socket.inproc_reconnect: %w", err)
				}
			} else {
				err := socket.reconnect()
				if err != nil {
					return nil, fmt.Errorf("socket.reconnect: %w", err)
				}
			}

			if attempt == 0 {
				return nil, fmt.Errorf("timeout")
			}
			attempt--
		}
	}
}

func (socket *ClientSocket) RequestRawMessage(requestString string) ([]string, error) {
	if socket.protocol == "inproc" {
		err := socket.inprocReconnect()
		if err != nil {
			return nil, fmt.Errorf("socket connection: %w", err)
		}

	} else {
		err := socket.reconnect()
		if err != nil {
			return nil, fmt.Errorf("socket connection: %w", err)
		}
	}

	requestTimeout := config.RequestTimeout(socket.appConfig)

	attempt := config.Attempt(socket.appConfig)

	// we attempt requests for an infinite amount of time.
	for {
		//  We send a request, then we work to get a reply
		if _, err := socket.socket.SendMessage(requestString); err != nil {
			return nil, fmt.Errorf("failed to send the message. socket error: %w", err)
		}

		// Poll socket for a reply, with timeout
		sockets, err := socket.poller.Poll(requestTimeout)
		if err != nil {
			return nil, fmt.Errorf("poll error: %w", err)
		}

		// Here we process a server reply and exit our loop if the
		//  reply is valid.
		// If we didn't have a reply, we close the client
		//  socket and resend the request.
		// We try a number of times
		//  before finally abandoning:

		if len(sockets) > 0 {
			// Wait for a reply.
			r, err := socket.socket.RecvMessage(0)
			if err != nil {
				return nil, fmt.Errorf("failed to message. socket error: %w", err)
			}

			return r, nil
		} else {
			socket.logger.Warn("timeout", "attempts_left", attempt)
			if socket.protocol == "inproc" {
				err := socket.inprocReconnect()
				if err != nil {
					return nil, fmt.Errorf("socket.inproc_reconnect: %w", err)
				}
			} else {
				err := socket.reconnect()
				if err != nil {
					return nil, fmt.Errorf("socket.reconnect: %w", err)
				}
			}

			if attempt == 0 {
				return nil, fmt.Errorf("timeout")
			}
			attempt--
		}
	}
}

// NewTcpSocket creates a new client socket over TCP protocol.
//
// The returned socket client then can send a message to server.Router and server.Reply
func NewTcpSocket(parent *log.Logger, appConfig *configEngine.Config) (*ClientSocket, error) {
	if appConfig == nil {
		return nil, fmt.Errorf("missing app_config")
	}

	sock, err := zmq.NewSocket(zmq.REQ)
	if err != nil {
		return nil, fmt.Errorf("zmq.NewSocket: %w", err)
	}

	var logger *log.Logger
	if parent != nil {
		logger = parent.Child("client_socket",
			"protocol", "tcp",
			"socket_type", "REQ",
		)
	}
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	newSocket := ClientSocket{
		socket:    sock,
		protocol:  "tcp",
		logger:    logger,
		appConfig: appConfig,
	}

	return &newSocket, nil
}

// NewReq creates a new client to connect to the service labeled as name (usually it's url)
//
// The returned socket client then can send a message to the server.Replier
func NewReq(name string, port uint64, parent *log.Logger) (*ClientSocket, error) {
	sock, err := zmq.NewSocket(zmq.REQ)
	if err != nil {
		return nil, fmt.Errorf("zmq.NewSocket: %w", err)
	}

	protocol := "tcp"
	if port == 0 {
		protocol = "inproc"
	}

	var logger *log.Logger
	if parent != nil {
		logger = parent.Child("client",
			"service name", name,
			"protocol", protocol,
			"socket_type", "REQ",
			"remote_service_url", ClientUrl(name, port),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	newSocket := ClientSocket{
		socket:      sock,
		protocol:    protocol,
		logger:      logger,
		appConfig:   nil,
		serviceName: name,
		servicePort: port,
	}

	return &newSocket, nil
}

// InprocRequestSocket creates a client socket with inproc protocol.
// The created client socket can connect to server.Router or server.Reply.
//
// The `url` request must start with `inproc://`
func InprocRequestSocket(url string, parent *log.Logger, appConfig *configEngine.Config) (*ClientSocket, error) {
	if appConfig == nil {
		return nil, fmt.Errorf("missing app_config")
	}

	if len(url) < 9 {
		return nil, fmt.Errorf("the url is too short")
	}
	if url[:9] != "inproc://" {
		return nil, fmt.Errorf("url doesn't start with `inproc` protocol. Its %s", url[:9])
	}

	sock, err := zmq.NewSocket(zmq.REQ)
	if err != nil {
		return nil, fmt.Errorf("zmq.NewSocket: %w", err)
	}

	logger := parent.Child("client_socket",
		"protocol", "inproc",
		"socket_type", "REQ",
		"remote_service_url", url)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	newSocket := ClientSocket{
		socket:   sock,
		protocol: "inproc",
		// client_credentials: nil,
		logger:    logger,
		appConfig: appConfig,
	}

	return &newSocket, nil
}

// ClientUrl creates url of the server for the client to connect
//
// If the port is 0, then the client will be inproc, not as tcp
func ClientUrl(name string, port uint64) string {
	if port == 0 {
		return fmt.Sprintf("inproc://%s", name)
	}
	url := fmt.Sprintf("tcp://localhost:%d", port)
	return url
}
