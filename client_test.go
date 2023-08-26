package client

import (
	"fmt"
	"github.com/ahmetson/client-lib/config"
	zmq "github.com/pebbe/zmq4"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// We won't test the requests.
// The requests are tested in the controllers
// Define the suite, and absorb the built-in basic suite
// functionality from test suite - including a T() method which
// returns the current testing orchestra
type TestClientSuite struct {
	suite.Suite

	socket  *Socket
	backend *zmq.Socket
}

func (test *TestClientSuite) SetupTest() {
	require := test.Require

	socket, err := NewRaw(zmq.ROUTER, "inproc://sample_router")
	require().NoError(err)

	test.socket = socket
}

func (test *TestClientSuite) TearDownTest() {
	require := test.Require

	if test.backend != nil {
		require().NoError(test.backend.Close())
	}

	if test.socket.zmqSocket != nil {
		require().NoError(test.socket.Close())
	}
}

// runBackend to imitate the backend as a goroutine.
func (test *TestClientSuite) runBackend(url string, zmqType zmq.Type) {
	fmt.Printf("backend running...\n")
	require := test.Require

	var err error
	test.backend, err = zmq.NewSocket(zmqType)
	require().NoError(err)

	fmt.Printf("backend bind to %s\n", url)
	err = test.backend.Bind(url)
	require().NoError(err)

	fmt.Printf("backend waits for messages...\n")
	msg, err := test.backend.RecvMessage(0)
	require().NoError(err)

	fmt.Printf("backend received: %s\n", msg)
	var reply []string
	if len(msg) >= 3 {
		reply = []string{msg[0], msg[1], fmt.Sprintf("reply to '%s'", msg[2])}
	} else if len(msg) >= 2 {
		reply = []string{msg[0], fmt.Sprintf("reply to '%s'", msg[1])}
	} else {
		reply = []string{fmt.Sprintf("reply to '%s'", msg[0])}
	}

	fmt.Printf("backend replies: %s\n", reply)

	_, err = test.backend.SendMessage(reply)
	require().NoError(err)

	fmt.Printf("backend replied: %s\n", reply)
	err = test.backend.Close()
	require().NoError(err)

	fmt.Printf("backed closed!\n")

	test.backend = nil
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

// Test_11_Parameters tests setting socket parameters such as timeout and attempts
func (test *TestClientSuite) Test_11_Parameters() {
	require := test.Require

	timeout := time.Millisecond // less than min
	attempt := uint8(0)

	require().Less(timeout, minTimeout, "set less than minTimeout")
	require().Less(attempt, minAttempt, "set less than minAttempt")

	// Setting a value less than minimal value should set to min
	test.socket.Timeout(timeout).Attempt(attempt)

	// The timeout must be minimal values
	require().EqualValues(minTimeout, test.socket.timeout)
	require().EqualValues(minAttempt, test.socket.attempt)
}

// Test_12_SubmitRaw test submitting the message.
func (test *TestClientSuite) Test_12_submitRaw() {
	require := test.Require

	go test.runBackend(test.socket.url, test.socket.target)

	req := "hello"
	err := test.socket.rawSubmit(req)
	require().NoError(err)
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestClient(t *testing.T) {
	suite.Run(t, new(TestClientSuite))
}
