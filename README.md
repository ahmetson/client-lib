# Client Lib
This is the client module.
The client is the socket that connects to the 
Handler Instances.

## Rules
* Client has options
* - *timeout* &ndash; of the request in milliseconds
* - *attempt* &ndash; of the request.
* Client must set correct message parts for asynchronous handlers.
* Client should request and submit

## Terminology

*Request* &ndash; means a client sends the message.
Then, waits for a reply.

*Submit* &ndash; means a client sends the message.
But doesn't wait for a reply.

## Implementation

### Options
Clients have default options for *timeout* and *attempt*.

The default options are hardcoded.
The default *timeout* is **10000 Milliseconds**.
The default *attempt* is **5**.

A client has `Timeout(milliseconds)` method
that overwrites the default timeout. The minimum
milliseconds for timeout are **2 milliseconds**. 
If the timeout is set to 1 or less than 1 millisecond,
the timeout will set it to minimum value.

A client has `Attempt()` method that overwrites the
default attempt. The minimum attempt is *1*. If
an attempt is set to 0 or less than that, the attempt
will set the minimum value.

### Type
The type of the client is derived from the combination
of the request socket and reply sockets.
