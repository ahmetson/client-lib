package client

import (
	"github.com/ahmetson/client-lib/config"
	zmq "github.com/pebbe/zmq4"
	"testing"

	"github.com/stretchr/testify/suite"
)

// We won't test the requests.
// The requests are tested in the controllers
// Define the suite, and absorb the built-in basic suite
// functionality from test suite - including a T() method which
// returns the current testing orchestra
type TestClientSuite struct {
	suite.Suite

	socket *Socket
}

func (test *TestClientSuite) SetupTest() {
	require := test.Require

	socket, err := NewRaw(zmq.ROUTER, "inproc://sample_router")
	require().NoError(err)

	test.socket = socket
}

// Test_10_New tests creation of the client
func (test *TestClientSuite) Test_10_New() {
	require := test.Require

	serviceUrl := "github.com/ahmetson/service"
	id := "sample_router"
	port := uint64(0)
	socketType := zmq.ROUTER
	client := config.New(serviceUrl, id, port, socketType)

	// Must fail, since config can not generate url
	_, err := New(client)
	require().Error(err)

	// Client is corrected
	client.UrlFunc(config.Url)
	_, err = New(client)
	require().NoError(err)
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestClient(t *testing.T) {
	suite.Run(t, new(TestClientSuite))
}
