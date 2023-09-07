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
	if err == zmq.ErrorSocketClosed {
		return
	}
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

	fmt.Printf("backend closed!\n")

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

// Test_12_rawSubmit test submitting the message.
func (test *TestClientSuite) Test_12_rawSubmit() {
	require := test.Require

	go test.runBackend(test.socket.url, test.socket.target)
	// Wait a bit for initialization
	time.Sleep(time.Millisecond * 100)

	req := "hello Test_12_rawSubmit"
	err := test.socket.rawSubmit(req)
	require().NoError(err)
}

// Test_13_RawRequest test requesting data.
func (test *TestClientSuite) Test_13_RawRequest() {
	require := test.Require

	go test.runBackend(test.socket.url, test.socket.target)
	// Wait a bit for initialization
	time.Sleep(time.Millisecond * 100)

	req := "hello Test_13_RawRequest"
	reply, err := test.socket.RawRequest(req)
	require().NoError(err)
	fmt.Printf("client recevied: %s\n", reply)
}

// Test_14_RawSubmit test submitting the message.
func (test *TestClientSuite) Test_14_RawSubmit() {
	require := test.Require

	go test.runBackend(test.socket.url, test.socket.target)

	req := "hello Test_14_RawSubmit"
	err := test.socket.RawSubmit(req)
	require().NoError(err)
}

// Test_15_DealerRawRequest test requesting data in asynchronous way.
func (test *TestClientSuite) Test_15_DealerRawRequest() {
	require := test.Require

	go test.runBackend(test.socket.url, test.socket.target)
	time.Sleep(time.Millisecond * 100)

	socket, err := NewRaw(zmq.ROUTER, "inproc://sample_router")
	require().NoError(err)
	test.socket = socket
	test.socket.socketType = zmq.DEALER

	// Set minimal timeout and attempt for fast testing
	test.socket.Timeout(time.Second).Attempt(minAttempt)

	req := "hello Test_15_DealerRawRequest"
	reply, err := test.socket.RawRequest(req)
	require().NoError(err)
	fmt.Printf("client recevied: %s\n", reply)

}

// Test_16_DealerRawSubmit test submitting the message in request way.
func (test *TestClientSuite) Test_16_DealerRawSubmit() {
	require := test.Require

	go test.runBackend(test.socket.url, test.socket.target)
	time.Sleep(time.Millisecond * 100)

	socket, err := NewRaw(zmq.ROUTER, "inproc://sample_router")
	require().NoError(err)
	test.socket = socket
	test.socket.socketType = zmq.DEALER
	// Set minimal timeout and attempt for fast testing
	test.socket.Timeout(time.Second).Attempt(minAttempt)

	req := "hello Test_16_DealerRawSubmit"
	err = test.socket.RawSubmit(req)
	require().NoError(err)

	// The backend closed
	time.Sleep(time.Millisecond * 10)
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestClient(t *testing.T) {
	suite.Run(t, new(TestClientSuite))
}
