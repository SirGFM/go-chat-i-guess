/*
Package go_chat_i_guess implements a generic, connection-agnostic chat server.

The chat is divided into three components:

 - `ChatServer`: The interface for the actual server
 - `ChatChannel`: The interface for a given chat channel/room
 - `Conn`: A connection to the remote client

Internally, there's also a fourth component, the `user`, but that's never
exported by the API. The user associates a `Conn` to a username, which must
be unique on that `ChatChannel`.

The first step to start a Chat Server is to instantiate it through one of
`NewServer`, `NewServerWithTimeout` or `NewServerConf`. The last one should
be the preferred variant, as it's the one that allows the most
customization:

    conf := go_chat_i_guess.GetDefaultServerConf()
    // Modify 'conf' as desired
    server := go_chat_i_guess.NewServerConf(conf)

A `ChatServer` by itself doesn't do anything. It simply manages
`ChatChannels` and connection tokens. Instantiating a new `ChatServer` will
start its cleanup goroutine, so idle `ChatChannel`s and expired tokens may
be automatically released (more on those in a moment).

`ChatChannel`s may be created on that `ChatServer`. Each channel is
uniquely identified by its name and runs its own goroutine to broadcast
messages to every connect user.

    // This should only fail if the channel's name has already been taken
    err := server.CreateChannel("channel-name")
    if err != nil {
        // Handle the error
    }

By default, a channel must receive a connection within 5 minutes of its
creation, otherwise it will automatically close. Two things are required to
connect a remote client to a channel: a authentication token and a
connection.

Since the `ChatServer` doesn't implement any authentication mechanism, the
caller is responsible by determining whether the requester is allowed to
connect to a given channel with the requested username. If the caller
accepts the requester's authentication (whichever it may be), it must
generate a token within the `ChatServer` for that username/channel pair:

    // XXX: Authenticate the user somehow

    token, err := server.RequestToken("the-user", "channel-name")
    if err != nil {
        // Handle the error
    }

    // XXX: Return this token to the requester

Then, the user must connect to the `ChatServer` using the retrieved token
and something that implements the `Conn` interface. `conn_test.c`
implements `mockConn`, which uses chan string to send and receive messages.
Another option could be to implement an interface for a WebSocket
connection. The user is added to the channel by calling either `Connect`,
which spawns a goroutine to wait for messages from the user, or
`ConnectAndWait`, which blocks until the `Conn` gets closed. This second
options may be useful if the server already spawns a goroutine to handle
requests.

    var conn Conn
    err := server.Connect(token, conn)
    if err != nil {
        // Handle the error
    }

From this point onward, `Conn.Recv` blocks waiting for a message, which
then gets forwarded to the `ChatChannel`. The `ChatChannel` then broadcasts
this message to every connected user, including the sender, by calling
their `Conn.SendStr`.

It's important to note that the although any `Conn` may be used for the
connection, it's also used to identify the user within the `ChatChannel`.
This may either be done by using a transport that maintains a connection
(TCP, WebSocket etc) or by manually associating this `Conn` to its
associated user.
*/
package go_chat_i_guess
