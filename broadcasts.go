package main

import (
	"github.com/hashicorp/memberlist"
	"log"
)

func newChatBroadcast(msg message) memberlist.Broadcast {
	data := msg.Encode()
	return &chatMsg{message: msg, data: data}
}

type chatMsg struct {
	message message
	data    []byte
}

// Invalidates checks if enqueuing the current broadcast
// invalidates a previous broadcast
func (bdcst *chatMsg) Invalidates(b memberlist.Broadcast) bool {
	other := new(message)
	if err := other.Decode(b.Message()); err != nil {
		return false
	}
	invalidate := other.UUID == bdcst.message.UUID
	if invalidate {
		log.Printf("invalidated: self=%v, other=%v", bdcst.message, other)
	}
	return invalidate
}

// Returns a byte form of the message
func (bdcst *chatMsg) Message() []byte { return bdcst.data }

// Finished is invoked when the message will no longer
// be broadcast, either due to invalidation or to the
// transmit limit being reached
func (bdcst *chatMsg) Finished() {}
