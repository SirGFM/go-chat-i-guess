package go_chat_i_guess

// Error type for this package.
type ChatError uint

const (
    // Invalid token. Either the token doesn't exist, it has already been used
    // or it has already expired.
    InvalidToken ChatError = iota
    // Channel did not receive any connections in a timely manner.
    IdleChannel
)

func (c ChatError) Error() string {
    switch c {
    case InvalidToken:
        return "Invalid token"
    case IdleChannel:
        return "Channel did not receive any connections in a timely manner"
    default:
        return "Unknown error"
    }
}
