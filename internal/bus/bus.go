package bus

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MessageType int

const (
	MsgUserInput    MessageType = iota
	MsgUserReply
	MsgTaskAssign
	MsgTaskResult
	MsgTaskFailed
	MsgNeedClarify
	MsgProgress
	MsgApprovalReq
	MsgApprovalResp
	MsgAllTasksDone
	MsgShutdown
)

type Message struct {
	ID        string
	From      string
	To        string
	Type      MessageType
	Payload   any
	ReplyTo   string
	Timestamp time.Time
}

type MessageBus struct {
	mu      sync.RWMutex
	subs    map[string]chan Message
	pending map[string]chan Message
}

func New() *MessageBus {
	return &MessageBus{
		subs:    make(map[string]chan Message),
		pending: make(map[string]chan Message),
	}
}

func (b *MessageBus) Subscribe(name string) <-chan Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Message, 32)
	b.subs[name] = ch
	return ch
}

func (b *MessageBus) Send(msg Message) {
	if msg.ID == "" {
		msg.ID = uuid.New().String()[:8]
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	b.mu.RLock()
	ch, ok := b.subs[msg.To]
	b.mu.RUnlock()

	if !ok {
		return
	}

	select {
	case ch <- msg:
	default:
	}

	if msg.ReplyTo != "" {
		b.mu.RLock()
		replyCh, ok := b.pending[msg.ReplyTo]
		b.mu.RUnlock()
		if ok {
			select {
			case replyCh <- msg:
			default:
			}
		}
	}
}

func (b *MessageBus) SendAndWait(msg Message, timeout time.Duration) (Message, error) {
	if msg.ID == "" {
		msg.ID = uuid.New().String()[:8]
	}

	replyCh := make(chan Message, 1)
	b.mu.Lock()
	b.pending[msg.ID] = replyCh
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.pending, msg.ID)
		b.mu.Unlock()
	}()

	b.Send(msg)

	select {
	case reply := <-replyCh:
		return reply, nil
	case <-time.After(timeout):
		return Message{}, fmt.Errorf("timeout waiting for reply to %s", msg.ID)
	}
}
