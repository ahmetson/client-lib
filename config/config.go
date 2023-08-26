// Package config is the utility function that keeps the client-server
package config

import (
	"fmt"
	zmq "github.com/pebbe/zmq4"
)

// A Client parameters to connect to the dep
type Client struct {
	ServiceUrl string // Url link of the service
	Id         string
	Port       uint64
	TargetType zmq.Type // The service's socket type
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
