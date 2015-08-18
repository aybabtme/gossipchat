package main

import (
	"log"
	"sort"
	"sync"

	"github.com/hashicorp/memberlist"
)

const (
	maxMessageHistory = 100
)

// delegate hooks into the gossip layer of Memberlist. All the methods
// must be thread-safe, as they can and generally will be called
// concurrently.
type delegate struct {
	mu        sync.Mutex
	msgs      *messageDB
	otherMsgc chan []message

	queue *memberlist.TransmitLimitedQueue
}

func newDelegate(history int, incoming <-chan message) (*delegate, <-chan []message) {
	del := &delegate{
		msgs:      newMessageDB(history),
		otherMsgc: make(chan []message, history),
		queue: &memberlist.TransmitLimitedQueue{
			NumNodes: func() int { return 1 },
		},
	}
	go func() {
		for msg := range incoming {
			del.msgs.Put(msg)
			del.queue.QueueBroadcast(newChatBroadcast(msg))
			go func() { del.otherMsgc <- del.msgs.Messages() }()
		}
	}()
	return del, del.otherMsgc
}

// NodeMeta is used to retrieve meta-data about the current node
// when broadcasting an alive message. It's length is limited to
// the given byte size. This metadata is available in the Node structure.
func (del *delegate) NodeMeta(limit int) []byte {
	return nil
}

// NotifyMsg is called when a user-data message is received.
// Care should be taken that this method does not block, since doing
// so would block the entire UDP packet receive loop. Additionally, the byte
// slice may be modified after the call returns, so it should be copied if needed.
func (del *delegate) NotifyMsg(msgraw []byte) {
	msg := new(message)
	err := msg.UnmarshalJSON(msgraw)
	if err != nil {
		log.Printf("error decoding message: %v", err)
		return
	}

	go func() {
		del.mu.Lock()
		defer del.mu.Unlock()
		del.msgs.Put(*msg)
		del.otherMsgc <- del.msgs.Messages()
	}()
}

// GetBroadcasts is called when user data messages can be broadcast.
// It can return a list of buffers to send. Each buffer should assume an
// overhead as provided with a limit on the total byte size allowed.
// The total byte size of the resulting data to send must not exceed
// the limit.
func (del *delegate) GetBroadcasts(overhead, limit int) [][]byte {
	del.mu.Lock()
	defer del.mu.Unlock()
	return del.queue.GetBroadcasts(overhead, limit)
}

// LocalState is used for a TCP Push/Pull. This is sent to
// the remote side in addition to the membership information. Any
// data can be sent here. See MergeRemoteState as well. The `join`
// boolean indicates this is for a join instead of a push/pull.
func (del *delegate) LocalState(join bool) []byte {
	del.mu.Lock()
	defer del.mu.Unlock()
	return encodeMessageDB(del.msgs)
}

// MergeRemoteState is invoked after a TCP Push/Pull. This is the
// state received from the remote side and is the result of the
// remote side's LocalState call. The 'join'
// boolean indicates this is for a join instead of a push/pull.
func (del *delegate) MergeRemoteState(buf []byte, join bool) {

	del.mu.Lock()
	defer del.mu.Unlock()
	if err := del.msgs.mergeRemoteDB(buf); err != nil {
		log.Printf("decoding message DB from MergeRemoteState()=%v", err)
		return
	}
	del.otherMsgc <- del.msgs.Messages()
}

// events is a simpler delegate that is used only to receive
// notifications about members joining and leaving. The methods
// in this delegate may be called by multiple goroutines, but never
// concurrently. This allows you to reason about ordering.
type events struct {
	members map[string]struct{}
	sendc   chan []string
}

func newEvents() (*events, <-chan []string) {
	events := &events{
		members: make(map[string]struct{}),
		sendc:   make(chan []string, 10),
	}
	return events, events.sendc
}

// NotifyJoin is invoked when a node is detected to have joined.
// The Node argument must not be modified.
func (del *events) NotifyJoin(nd *memberlist.Node) {
	if _, ok := del.members[nd.Name]; ok {
		return
	}
	del.members[nd.Name] = struct{}{}
	del.sendMembers()
}

// NotifyLeave is invoked when a node is detected to have left.
// The Node argument must not be modified.
func (del *events) NotifyLeave(nd *memberlist.Node) {
	if _, ok := del.members[nd.Name]; !ok {
		return
	}
	delete(del.members, nd.Name)
	del.sendMembers()
}

func (del *events) sendMembers() {
	var members []string
	for member := range del.members {
		members = append(members, member)
	}
	sort.Strings(members)
	select {
	case del.sendc <- members:
	default:
	}
}

// NotifyUpdate is invoked when a node is detected to have
// updated, usually involving the meta data. The Node argument
// must not be modified.
func (del *events) NotifyUpdate(nd *memberlist.Node) {
	panic("not implemented")
}

// conflicts is a used to inform a client that a node has attempted
// to join which would result in a name conflict. This happens if two
// clients are configured with the same name but different addresses.
type conflicts struct{}

func newConflicts() *conflicts { return new(conflicts) }

// NotifyConflict is invoked when a name conflict is detected
func (del *conflicts) NotifyConflict(existing, other *memberlist.Node) {
}
