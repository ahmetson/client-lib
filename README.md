# Client Lib
The client allows a connection to the Handlers.

## Terminology
*Send* &ndash; a message transfer between a client and handler.

*Request* &ndash; a client sends the message to the handler.
Then, waits for a reply.
It guarantees a delivery.

*Submit* &ndash; a client sends the message to the handler without waiting for a reply. 
It doesn't guarantee a delivery.

## Rules
* Client has options
* - *timeout* &ndash; option that halts sending after this amount. Minimum value is 2 milliseconds.
* - *attempt* &ndash; option that sending will be tried after *timeout*. Minimum one attempt.
* Client must set correct message parts for asynchronous handlers for internal zeromq socket.
* Client must implement *request* and *submit*.
* Client must be used from multiple threads.

## Implementation

### Options
The default options are hardcoded.
The default *timeout* is **10 Seconds**.
The default *attempt* is **5**.

The `Timeout(time.Duration)` method sets the timeout. 
The `minimumTimeout` is **2 milliseconds**. 

The `Attempt(uint8)` method sets the attempt. 
The minimum attempt is *1*. 

### Type
The type of the client is derived from the combination
of the request socket and reply sockets.

The type is derived from the target type.

### Multi thread
Suppose we defined a client for extension.
Then the extension client is passed to the handler function.
Let's think that we have 20 concurrent requests.
Each of the requests will use the client to send a message to the extension.

The submit function adds the user request to the queue with the channel.
The submitting waits for a channel.
The second concurrent request adds the message to the queue.
Then wait for a reply in the channel.

The client has the consumer. 
The consumer checks the queue.
If the message is given in the queue, then sends.
If the message has the channel, then reply back.
And returns result in the channel.