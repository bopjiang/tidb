package pgserver

// MessageType is message type
type MessageType byte

// MessageType defines
const (
	MessageTypeInvalid               MessageType = 0
	MessageTypeAuthenticationRequest MessageType = 'R' //B
	MessageTypePasswordMessage       MessageType = 'p' //F
	MessageTypeParameterStatus       MessageType = 'S' //B
	MessageTypeBackendKeyData        MessageType = 'K' //B
	MessageTypeReadyForQuery         MessageType = 'Z' //B
	MessageTypeSimpleQuery           MessageType = 'Q' //F

)
