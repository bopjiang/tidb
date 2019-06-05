package pgserver

// Copyright (c) 2018 Jiang Jia
// Apache License 2.0

// MessageType is message type
type MessageType byte

// MessageType defines
const (
	MessageTypeInvalid               MessageType = 0
	MessageTypeAuthenticationRequest MessageType = 'R' //B: backend
	MessageTypePasswordMessage       MessageType = 'p' //F: frontend
	MessageTypeParameterStatus       MessageType = 'S' //B
	MessageTypeBackendKeyData        MessageType = 'K' //B
	MessageTypeReadyForQuery         MessageType = 'Z' //B
	MessageTypeSimpleQuery           MessageType = 'Q' //F
	MessageTypeRowDescription        MessageType = 'T' //B
	MessageTypeDataRow               MessageType = 'D' //B
	MessageTypeCommandComplete       MessageType = 'C' //B
	MessageTypeEmptyQueryResponse    MessageType = 'I' //B
)
