package go_chat_i_guess

import (
    "strings"
    "testing"
    "time"
    "runtime"
)

// TestAccessToken check whether the access token is correctly evicted after its
// deadline and after being used.
func TestAccessToken(t *testing.T) {
    const bufSize = 128
    const tokenDeadline = time.Millisecond * 2
    const tokenCleanupDelay = time.Millisecond * 20

    s := NewServerWithTimeout(bufSize, bufSize, tokenDeadline, tokenCleanupDelay)

    // Check that the server was correctly configured.
    conf := s.GetConf()
    if want, got := bufSize, conf.ReadBuf; want != got {
        t.Errorf("Invalid ReadBuf retrieved: expected '%d' but got '%d'", want, got)
    } else if want, got := bufSize, conf.WriteBuf; want != got {
        t.Errorf("Invalid WriteBuf retrieved: expected '%d' but got '%d'", want, got)
    } else if want, got := tokenDeadline, conf.TokenDeadline; want != got {
        t.Errorf("Invalid TokenDeadline retrieved: expected '%d' but got '%d'", want, got)
    } else if want, got := tokenCleanupDelay, conf.TokenCleanupDelay; want != got {
        t.Errorf("Invalid TokenCleanupDelay retrieved: expected '%d' but got '%d'", want, got)
    }

    // Retrieve a reference to the internal server, to check the token storage.
    _s := s.(*server)

    // Try to generate a access token and retrieve it within the deadline.
    tk, err := s.RequestToken("user", "chan")
    if err != nil {
        t.Errorf("Couldn't generate the request token: %+v", err)
    }
    time.Sleep(tokenDeadline / 2)

    username, channel, err := _s.getToken(tk)
    if err != nil {
        t.Errorf("Couldn't retrieve a token before it expired: %+v", err)
    } else if want, got := "user", username; want != got {
        t.Errorf("Invalid user retrieved: expected '%s' but got '%s'", want, got)
    } else if want, got = "chan", channel; want != got {
        t.Errorf("Invalid channel retrieved: expected '%s' but got '%s'", want, got)
    }

    // Try to get the token once again and ensure that it fails.
    _, _, err = _s.getToken(tk)
    if err == nil {
        t.Error("Successfully got a previously consumed")
    } else if got, ok := err.(ChatError); !ok {
        t.Errorf("Invalid error! A 'ChatError' but got '%+v'", err)
    } else if want := InvalidToken; want != got {
        t.Errorf("Invalid error! Expected '%+v' but got '%+v'", want, got)
    }

    // Try to generate another access token and retrieve it after it's expired.
    tk, err = s.RequestToken("user", "chan")
    if err != nil {
        t.Errorf("Couldn't generate the request token: %+v", err)
    }
    time.Sleep(tokenCleanupDelay + tokenCleanupDelay / 2)

    _, _, err = _s.getToken(tk)
    if err == nil {
        t.Error("Successfully got an expired token")
    } else if got, ok := err.(ChatError); !ok {
        t.Errorf("Invalid error! A 'ChatError' but got '%+v'", err)
    } else if want := InvalidToken; want != got {
        t.Errorf("Invalid error! Expected '%+v' but got '%+v'", want, got)
    }
}

// TestChannel check whether a channel is correctly evicted after getting
// closed.
func TestChannel(t *testing.T) {
    conf := GetDefaultServerConf()
    conf.ChannelCleanupDelay = time.Millisecond * 10

    s := NewServerConf(conf)

    // Try to retrieve a non-existing channel.
    _, err := s.GetChannel("chan")
    if err == nil {
        t.Error("Successfully got a non-existing channel")
    } else if got, ok := err.(ChatError); !ok {
        t.Errorf("Invalid error! Expected a 'ChatError' but got '%+v'", err)
    } else if want := InvalidChannel; want != got {
        t.Errorf("Invalid error! Expected '%+v' but got '%+v'", want, got)
    }

    // Create a channel, check that it's unique and try to retrieve it.
    err = s.CreateChannel("chan")
    if err != nil {
        t.Errorf("Failed to create a channel: %+v", err)
    }
    err = s.CreateChannel("chan")
    if err == nil {
        t.Error("Successfully created a duplicated channel")
    } else if got, ok := err.(ChatError); !ok {
        t.Errorf("Invalid error! Expected a 'ChatError' but got '%+v'", err)
    } else if want := DuplicatedChannel; want != got {
        t.Errorf("Invalid error! Expected '%+v' but got '%+v'", want, got)
    }

    c, err := s.GetChannel("chan")
    if err != nil {
        t.Errorf("Couldn't get the created channel: %+v", err)
    }

    // Check that the channel gets evicted after its timeout.
    c.Close()
    time.Sleep(conf.ChannelCleanupDelay + conf.ChannelCleanupDelay / 2)
    _, err = s.GetChannel("chan")
    if err == nil {
        t.Error("Successfully got an expired channel")
    } else if got, ok := err.(ChatError); !ok {
        t.Errorf("Invalid error! Expected a 'ChatError' but got '%+v'", err)
    } else if want := InvalidChannel; want != got {
        t.Errorf("Invalid error! Expected '%+v' but got '%+v'", want, got)
    }

    s.Close()
}

type expectedMsg struct {
    conn *mockConn
    name string
    msg string
}

type expectedReceiver struct {
    conn *mockConn
    name string
}

// TestConn check whether messages are correctly sent to/from users.
func TestConn(t *testing.T) {
    const u1 = "user1"
    const u2 = "user2"
    const cn = "chan"

    conf := GetDefaultServerConf()
    conf.ChannelCleanupDelay = time.Millisecond * 5

    // Create connections to be used by clients
    c1 := NewMockConn()
    _c1 := c1.(*mockConn)
    c2 := NewMockConn()
    _c2 := c2.(*mockConn)

    s := NewServerConf(conf)

    // Create a channel, connect a user and check that the channel doesn't
    // get evicted after the timeout
    err := s.CreateChannel(cn)
    if err != nil {
        t.Errorf("Failed to create a channel: %+v", err)
    }

    tk, err := s.RequestToken(u1, cn)
    if err != nil {
        t.Errorf("Failed to create a connection token for %s and %s: %+v", u1, c1, err)
    }
    err = s.Connect(tk, c1)
    if err != nil {
        t.Errorf("Failed to connect %s to %s: %+v", u1, c1, err)
    }

    time.Sleep(conf.ChannelCleanupDelay + conf.ChannelCleanupDelay / 2)
    _, err = s.GetChannel(cn)
    if err != nil {
        t.Error("Channel timedout!")
    }

    // Chech that u1 receive the message about themselves joining
    m, err := _c1.TestRecv(time.Millisecond * 5)
    if err != nil {
        t.Errorf("%s failed to detected that %s joined %s: %+v", u1, u1, cn, err)
    } else if !strings.Contains(m, u1) {
        t.Errorf("Message does not say that %s joined", u1)
    }

    // Connect another user, so they may communicate
    tk, err = s.RequestToken(u2, cn)
    if err != nil {
        t.Errorf("Failed to create a connection token for %s and %s: %+v", u2, c2, err)
    }
    err = s.Connect(tk, c2)
    if err != nil {
        t.Errorf("Failed to connect %s to %s: %+v", u2, c2, err)
    }

    // Check that both users received a message about u2 joining
    receivers := []expectedReceiver {
        { _c1, u1 },
        { _c2, u2 },
    }
    for _, recv := range receivers {
        msg, err := recv.conn.TestRecv(time.Millisecond * 5)
        if err != nil {
            t.Errorf("%s failed to detected that %s joined %s: %+v", recv.name, u2, cn, err)
        } else if !strings.Contains(msg, u2) {
            t.Errorf("Message does not say that %s joined\n\tGot: %s", u2, msg)
        }
    }

    // Send a few messages and check that they arrive on both ends,
    // sequentially.
    //
    // Text: Jabberwocky by Lewis Carroll.
    input := []expectedMsg {
        { conn: _c1, name: u1, msg: "Twas brillig, and the slithy toves" },
        { conn: _c1, name: u1, msg: "Did gyre and gimble in the wabe;" },
        { conn: _c2, name: u2, msg: "All mimsy were the borogoves," },
        { conn: _c1, name: u1, msg: "And the mome raths outgrabe." },
    }
    for _, in := range input {
        err := in.conn.TestSend(in.msg)
        if err != nil {
            t.Errorf("Failed to send the message '%s' from %s: %+v", in.msg, in.name, err)
        }
        // Let other goroutines execute, so we don't throttle the server
        // and send things out of order.
        runtime.Gosched()
    }
    for _, in := range input {
        for _, recv := range receivers {
            msg, err := recv.conn.TestRecv(time.Millisecond * 5)
            if err != nil {
                t.Errorf("%s failed to receive the message '%s' from %s: %+v", recv.name, in.msg, in.name, err)
            } else if !strings.Contains(msg, in.name) {
                t.Errorf("Message does not say it was sent by %s:\n\tgot: %s", in.name, msg)
            } else if !strings.Contains(msg, in.msg) {
                t.Errorf("Message does not contain the expected text:\n\twant: %s\n\tgot: %s", in.msg, msg)
            }
        }
    }

    // Check the channel's name and the list of users
    c, err := s.GetChannel(cn)
    if err != nil {
        t.Errorf("Couldn't retrieve the channel: %+v", err)
    } else if want, got := cn, c.Name(); want != got {
        t.Errorf("Channel has an invalid name! Expected '%s' but got '%s'", want, got)
    }
    userList := c.GetUsers(nil)
    for _, want := range []string { u1, u2 } {
        var found bool
        for _, got := range userList {
            if want == got {
                found = true
                break
            }
        }
        if !found {
            t.Errorf("User '%s' isn't on the user list (%+v)!", want, userList)
        }
    }

    s.Close()
}
