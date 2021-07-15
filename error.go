package go_chat_i_guess

// Error type for this package.
type ChatError uint

const (
    // Invalid token. Either the token doesn't exist, it has already been used
    // or it has already expired.
    InvalidToken ChatError = iota
    // Channel did not receive any connections in a timely manner.
    IdleChannel
    // There's already another channel with the requested name.
    DuplicatedChannel
    // Invalid Channel. Either the channel doesn't exist or it has already
    // expired (or been closed).
    InvalidChannel
    // The channel was closed before the operation completed.
    ChannelClosed
)

func (c ChatError) Error() string {
    switch c {
    case InvalidToken:
        return "Invalid token"
    case IdleChannel:
        return "Channel did not receive any connections in a timely manner"
    case DuplicatedChannel:
        return "There's already another channel with the requested name"
    case InvalidChannel:
        return "Invalid Channel"
    case ChannelClosed:
        return "The channel was closed before the operation completed"
    default:
        return "Unknown error"
    }
}
