package fix

import (
	"time"

	fix "github.com/quickfixgo/quickfix"
)

// NewNoStoreFactory returns a new NoStoreFactory
func NewNoStoreFactory() fix.MessageStoreFactory {
	return &noStoreFactory{}
}

type noStoreFactory struct {
}

func (nsf *noStoreFactory) Create(sessionID fix.SessionID) (fix.MessageStore, error) {
	store := new(noStore)
	store.Reset()
	return store, nil
}

type noStore struct {
	senderMsgSeqNum, targetMsgSeqNum int
	creationTime                     time.Time
}

func (n *noStore) NextSenderMsgSeqNum() int {
	return n.senderMsgSeqNum + 1
}

func (n *noStore) NextTargetMsgSeqNum() int {
	return n.targetMsgSeqNum + 1
}

func (n *noStore) IncrNextTargetMsgSeqNum() error {
	n.targetMsgSeqNum++
	return nil
}

func (n *noStore) IncrNextSenderMsgSeqNum() error {
	n.senderMsgSeqNum++
	return nil
}

func (n *noStore) SetNextSenderMsgSeqNum(num int) error {
	n.senderMsgSeqNum = num - 1
	return nil
}

func (n *noStore) SetNextTargetMsgSeqNum(num int) error {
	n.targetMsgSeqNum = num - 1
	return nil
}

func (n *noStore) CreationTime() time.Time {
	return n.creationTime
}

func (n *noStore) SaveMessage(seqNum int, msg []byte) error {
	return nil
}

func (n *noStore) GetMessages(beginSeqNum, endSeqNum int) ([][]byte, error) {
	return nil, nil
}

func (n *noStore) Refresh() error {
	return nil
}

func (n *noStore) Reset() error {
	n.senderMsgSeqNum = 0
	n.targetMsgSeqNum = 0
	n.creationTime = time.Now()
	return nil
}

func (n *noStore) Close() error {
	return nil
}
