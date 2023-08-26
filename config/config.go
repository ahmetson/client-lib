// Package config is the utility function that keeps the client-server
package config

import (
	"fmt"
	zmq "github.com/pebbe/zmq4"
)

// A Client parameters to connect to the dep
type Client struct {
	ServiceUrl string   `json:"url"` // Url link of the service
	Id         string   `json:"id"`
	Port       uint64   `json:"port"`
	TargetType zmq.Type `json:"zmq_type,omitempty"` // The service's socket type
	urlFunc    func(*Client) string
}

func New(url string, id string, port uint64, socketType zmq.Type) *Client {
	return &Client{
		ServiceUrl: url,
		Id:         id,
		Port:       port,
		TargetType: socketType,
		urlFunc:    nil,
	}
}

// UrlFunc sets the context to generate the url
func (client *Client) UrlFunc(urlFunc func(*Client) string) {
	client.urlFunc = urlFunc
}

// Url of the client
func (client *Client) Url() string {
	if client.urlFunc == nil {
		return ""
	}

	return client.urlFunc(client)
}

// Url creates url of the server for the client to connect
//
// If the port is 0, then the client will be inproc, not as tcp
// todo move to context
func Url(id string, port uint64) string {
	if port == 0 {
		return fmt.Sprintf("inproc://%s", id)
	}
	url := fmt.Sprintf("tcp://localhost:%d", port)
	return url
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
