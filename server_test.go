package go_chat_i_guess

import (
    "testing"
    "time"
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
