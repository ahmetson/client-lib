# Client Lib
The `client` module is to message exchange with [SDS Handlers](https://github.com/ahmetson/handler-lib/).

## Terminology
*Transmit* &ndash; any message transfers between a client and handler.

*Request* &ndash; is the two **transmits** with the handler. Sending and receiving. It guarantees a delivery.

*Submit* &ndash; a one **transmit** with the handler.
The client sends the message.
Client doesn't wait for a reply. 

*Target* &ndash; a handler to which a message is **transmitted**.

## Rules
* Client has options
* - *timeout* &ndash; option that halts sending after this period of time. Minimum value is 2 milliseconds.
* - *attempt* &ndash; option that repeats the message **transmitting** after *timeout*. Minimum one attempt.
* Client must set correct message parts for asynchronous handlers for internal zeromq socket.

## Implementation

### Options
The default *timeout* is **10 Seconds**.
The default *attempt* is **5**.

The `Client.Timeout(time.Duration)` method over-writes the timeout. 
The `minimumTimeout` is **2 milliseconds**. 

The `Client.Attempt(uint8)` method sets the attempt. 
The minimum attempt is *1*. 

### Type
The type of the client is the opposite of the target type.
Thus, when a client is defined, it's defined against the target to whom it will interact with.

The handlers use the clients for creating a managers.
To avoid import cycling the clients are using the target's internal socket type.

For intercommunication SDS framework uses Zeromq sockets. 

### Concurrent
A client is concurrent with its message queue.
A client is [thread safe](https://en.wikipedia.org/wiki/Thread_safety).

One client can send multiple messages at the same time.

```go
// Thread 1
client1.Request(message)
// Thread 2
client1.Request(message)
```

> **Todo**
> 
> Optimize the client passing to the handle functions as one child is passed to multiple handle func.
> We need to avoid passing from parent to the nested child tree.

> Test the limits of the clients, and number of the threads.
> Maybe create a pool of client sockets and get one when it's available?

> **Todo**
> 
> Create a library of the pool for available sockets. 
> Then design the handle and client based on that.

