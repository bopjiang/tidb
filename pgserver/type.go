package pgserver

// MessageType is message type
type MessageType byte

// MessageType defines
const (
	MessageTypeInvalid               MessageType = 0
	MessageTypeAuthenticationRequest MessageType = 'R' //B
	MessageTypePasswordMessage       MessageType = 'p' //F
)
