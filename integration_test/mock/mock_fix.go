package mock

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	fix "github.com/quickfixgo/quickfix"
)

// key = SessionID
type Session struct {
	ID       fix.SessionID
	LoggedOn bool
	Received map[int]string
	Sent     map[int]string
	m        sync.Mutex
	wg       sync.WaitGroup
}

func (s *Session) Send(msg fix.Messagable) {
	fix.SendToTarget(msg, s.ID)
}

type TestFixClient struct {
	initiator *fix.Initiator
	settings  *fix.Settings
	router    *fix.MessageRouter

	Sessions map[string]*Session
	last     *Session

	sendOnLogon []fix.Messagable

	OmitLogMessages bool
	MessageHandler

	ApiKey, ApiSecret, BfxUserID string
}

func (t *TestFixClient) Handle(msg *fix.Message) {
	// no-op
}

type MessageHandler interface {
	Handle(msg *fix.Message)
}

func (t *TestFixClient) SetMessageHandler(handler MessageHandler) {
	t.MessageHandler = handler
}

func (t *TestFixClient) Send(msg fix.Messagable) error {
	err := fix.SendToTarget(msg, t.last.ID)
	soh := byte(0x1)
	str := strings.Replace(msg.ToMessage().String(), string([]byte{soh}), "|", -1)
	log.Printf("[-> %s]: %s", t.last.ID.String(), str)
	return err
}

func (t *TestFixClient) WaitForSession(sid string) error {
	found := false
	for i := 0; i < 20; i++ {
		if session, ok := t.Sessions[sid]; ok {
			if session.LoggedOn {
				log.Printf("found session %s", sid)
				return nil
			}
			found = true
		}
		time.Sleep(time.Millisecond * 250)
	}
	for _, s := range t.Sessions {
		log.Printf("test fix client session: %s", s.ID.String())
	}
	if found {
		return fmt.Errorf("session found but not logged on: %s", sid)
	}
	return fmt.Errorf("could not find session: %s", sid)
}

func (t *TestFixClient) LastSession() *Session {
	return t.last
}

func NewTestFixClient(settings *fix.Settings, msgStore fix.MessageStoreFactory) (*TestFixClient, error) {
	f := &TestFixClient{
		router:   fix.NewMessageRouter(),
		settings: settings,
		Sessions: make(map[string]*Session),
	}
	f.MessageHandler = f
	logFactory, err := fix.NewFileLogFactory(settings)
	if err != nil {
		log.Print("could not create file log factory")
		return nil, err
	}
	initiator, err := fix.NewInitiator(f, msgStore, settings, logFactory)
	if err != nil {
		log.Printf("could not create acceptor: %s", err.Error())
		return nil, err
	}
	f.initiator = initiator
	return f, nil
}

func (m *TestFixClient) ReceivedCount(sessionID string) int {
	if s, ok := m.Sessions[sessionID]; ok {
		s.m.Lock()
		defer s.m.Unlock()
		return len(s.Received)
	}
	return 0
}

func (m *TestFixClient) WaitForMessage(sessionID string, seqnum int) (string, error) {
	if s, ok := m.Sessions[sessionID]; ok {
		wait := time.Duration(time.Second * 4)
		loops := 5
		delay := wait / time.Duration(loops)
		for i := 0; i < loops; i++ {
			s.m.Lock()
			if m, ok := s.Received[seqnum]; ok {
				s.m.Unlock()
				return m, nil
			}
			s.m.Unlock()
			time.Sleep(delay)
		}
		m.DumpRcvFIX(sessionID)
		return "", fmt.Errorf("did not receive message seqeuence number %d", seqnum)
	}
	return "", fmt.Errorf("could not find session %s", sessionID)
}

func (m *TestFixClient) DumpRcvFIX(sid string) {
	if s, ok := m.Sessions[sid]; ok {
		for i, m := range s.Received {
			log.Printf("%2d: %s", i, m)
		}
	}
}

func (m *TestFixClient) Start() error {
	return m.initiator.Start()
}

func (m *TestFixClient) Stop() {
	m.initiator.Stop()
	for _, s := range m.Sessions {
		err := fix.UnregisterSession(s.ID)
		if err != nil {
			log.Printf("[TEST] could not remove session %s: %s", s.ID.String(), err.Error())
		} else {
			log.Printf("[TEST] removed session: %s", s.ID.String())
		}
		counterparty := fix.SessionID{
			BeginString:      s.ID.BeginString,
			TargetCompID:     s.ID.SenderCompID,
			TargetSubID:      s.ID.SenderSubID,
			TargetLocationID: s.ID.SenderLocationID,
			SenderCompID:     s.ID.TargetCompID,
			SenderSubID:      s.ID.TargetSubID,
			SenderLocationID: s.ID.TargetLocationID,
			Qualifier:        s.ID.Qualifier,
		}
		err = fix.UnregisterSession(counterparty)
		if err != nil {
			log.Printf("[TEST] could not remove session %s: %s", counterparty.String(), err.Error())
		} else {
			log.Printf("[TEST] removed session: %s", counterparty.String())
		}
	}
}

func (m *TestFixClient) OnLogout(sessionID fix.SessionID) {
	log.Print("MockFix.OnLogout", sessionID)
	m.Sessions[sessionID.String()].LoggedOn = false
	return
}

func (m *TestFixClient) SendOnLogon(msgs []fix.Messagable) {
	m.sendOnLogon = msgs
}

func (m *TestFixClient) onLogon(sessionID fix.SessionID) {
	if m.sendOnLogon == nil {
		return
	}
	go func() {
		for _, msg := range m.sendOnLogon {
			fix.SendToTarget(msg, sessionID)
		}
	}()
}

func (m *TestFixClient) OnLogon(sessionID fix.SessionID) {
	log.Print("MockFix.OnLogon", sessionID)
	m.Sessions[sessionID.String()].LoggedOn = true
	m.onLogon(sessionID)
	return
}

func fixString(msg fix.Messagable) string {
	return strings.Replace(msg.ToMessage().String(), string([]byte{0x1}), "|", -1)
}

// outgoing admin
func (m *TestFixClient) ToAdmin(msg *fix.Message, sessionID fix.SessionID) {
	log.Print("MockFix.ToAdmin (outgoing)", fixString(msg))
	msgType, err := msg.MsgType()
	if err != nil {
		return
	}
	if "A" == msgType {
		msg.Body.SetString(fix.Tag(20000), m.ApiKey)
		msg.Body.SetString(fix.Tag(20001), m.ApiSecret)
		msg.Body.SetString(fix.Tag(20002), m.BfxUserID)
	}
	return
}

// incoming admin
func (m *TestFixClient) FromAdmin(msg *fix.Message, sID fix.SessionID) fix.MessageRejectError {
	log.Print("MockFix.FromAdmin (incoming)", fixString(msg))
	return nil
}

// outgoing app
func (m *TestFixClient) ToApp(msg *fix.Message, sID fix.SessionID) error {
	if !m.OmitLogMessages {
		log.Print("MockFix.ToApp (outgoing)", fixString(msg))
	}
	s := m.Sessions[sID.String()]
	seq, err := msg.Header.GetInt(34)
	if err != nil {
		return err
	}
	s.Sent[seq] = strings.Replace(msg.String(), string(0x1), "|", -1)
	return nil
}

// incoming app
func (m *TestFixClient) FromApp(msg *fix.Message, sID fix.SessionID) fix.MessageRejectError {
	if !m.OmitLogMessages {
		log.Print("MockFix.FromApp (incoming)", fixString(msg))
	}
	s := m.Sessions[sID.String()]
	s.m.Lock()
	defer s.m.Unlock()
	seq, err := msg.Header.GetInt(34)
	if err != nil {
		return err
	}
	s.Received[seq] = strings.Replace(msg.String(), string(0x1), "|", -1)
	m.MessageHandler.Handle(msg)
	return nil
}

func (m *TestFixClient) OnCreate(sessionID fix.SessionID) {
	log.Print("MockFix.OnCreate ", sessionID)
	s := &Session{
		ID:       sessionID,
		LoggedOn: false,
		Received: make(map[int]string),
		Sent:     make(map[int]string),
	}
	m.Sessions[sessionID.String()] = s
	m.last = s
	return
}
