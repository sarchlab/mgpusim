package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// The receiver task ID registry maps (domain, message-ID) to the local task ID
// that the receiver uses to track its handling of that message. This lets a
// receiver derive a stable task ID for an incoming message without mutating
// the message itself.

type receiverTaskKey struct {
	domain string
	msgID  uint64
}

var (
	receiverTaskIDs   = make(map[receiverTaskKey]uint64)
	receiverTaskIDsMu sync.Mutex
)

func lookupOrCreateReceiverTaskID(msg messaging.Msg, domain NamedHookable) uint64 {
	key := receiverTaskKey{domain: domain.Name(), msgID: msg.Meta().ID}

	receiverTaskIDsMu.Lock()
	defer receiverTaskIDsMu.Unlock()

	if id, ok := receiverTaskIDs[key]; ok {
		return id
	}

	id := timing.GetIDGenerator().Generate()
	receiverTaskIDs[key] = id

	return id
}

func forgetReceiverTaskID(msg messaging.Msg, domain NamedHookable) {
	forgetReceiverTaskIDByMsgID(msg.Meta().ID, domain)
}

func forgetReceiverTaskIDByMsgID(msgID uint64, domain NamedHookable) {
	key := receiverTaskKey{domain: domain.Name(), msgID: msgID}

	receiverTaskIDsMu.Lock()
	delete(receiverTaskIDs, key)
	receiverTaskIDsMu.Unlock()
}
