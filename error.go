package go_chat_i_guess

// Error type for this package.
type ChatError uint

const (
    // Invalid token. Either the token doesn't exist, it has already been used
    // or it has already expired.
    InvalidToken ChatError = iota
)

func (c ChatError) Error() string {
    switch c {
    case InvalidToken:
        return "Invalid token"
    default:
        return "Unknown error"
    }
}
