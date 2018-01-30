package mock

import (
	"fmt"
	fix "github.com/quickfixgo/quickfix"
	"log"
	"sync"
	"time"
)

// key = SessionID
type Session struct {
	ID       fix.SessionID
	LoggedOn bool
	Received []*fix.Message
	Sent     []*fix.Message
	m        sync.Mutex
	wg       sync.WaitGroup
}

func (s *Session) Send(msg fix.Messagable) {
	fix.SendToTarget(msg, s.ID)
}

type MockFIX struct {
	initiator *fix.Initiator
	settings  *fix.Settings
	router    *fix.MessageRouter

	Sessions map[string]*Session
	last     *Session

	ApiKey, ApiSecret, BfxUserID string
}

func (m *MockFIX) LastSession() *Session {
	return m.last
}

func (m *MockFIX) Send(msg fix.Messagable) {
	m.LastSession().Send(msg)
}

func NewMockFIX(settings *fix.Settings) (*MockFIX, error) {
	f := &MockFIX{
		router:   fix.NewMessageRouter(),
		settings: settings,
		Sessions: make(map[string]*Session),
	}
	logFactory, err := fix.NewFileLogFactory(settings)
	if err != nil {
		log.Print("could not create file log factory")
		return nil, err
	}
	initiator, err := fix.NewInitiator(f, fix.NewMemoryStoreFactory(), settings, logFactory)
	if err != nil {
		return nil, err
	}
	f.initiator = initiator
	return f, nil
}

func (m *MockFIX) ReceivedCount(sessionID string) int {
	if s, ok := m.Sessions[sessionID]; ok {
		s.m.Lock()
		defer s.m.Unlock()
		return len(s.Received)
	}
	return 0
}

func (m *MockFIX) WaitForMessage(sessionID string, num int) (*fix.Message, error) {
	if s, ok := m.Sessions[sessionID]; ok {
		wait := time.Duration(time.Second * 4)
		loops := 5
		delay := wait / time.Duration(loops)
		for i := 0; i < loops; i++ {
			s.m.Lock()
			defer s.m.Unlock()
			n := len(s.Received)
			if num > n {
				return s.Received[num], nil
			}
			time.Sleep(delay)
		}
		return nil, fmt.Errorf("did not receive message %d on session %s", num, sessionID)
	}
	return nil, fmt.Errorf("could not find session %s", sessionID)
}

func (m *MockFIX) DumpFix(sessionID string) {
	if s, ok := m.Sessions[sessionID]; ok {
		for _, msg := range s.Received {
			log.Printf("%#v", msg)
		}
	}
}

func (m *MockFIX) Start() error {
	return m.initiator.Start()
}

func (m *MockFIX) Stop() {
	m.initiator.Stop()

	for _, s := range m.Sessions {
		err := fix.UnregisterSession(s.ID)
		if err != nil {
			log.Printf("could not remove session %s: %s", s.ID.String(), err.Error())
		}
	}
}

func (m *MockFIX) OnLogout(sessionID fix.SessionID) {
	log.Print("MockFix.OnLogout", sessionID)
	m.Sessions[sessionID.String()].LoggedOn = false
	return
}

func (m *MockFIX) OnLogon(sessionID fix.SessionID) {
	log.Print("MockFix.OnLogon", sessionID)
	m.Sessions[sessionID.String()].LoggedOn = true
	return
}

func (m *MockFIX) ToAdmin(msg *fix.Message, sessionID fix.SessionID) {
	log.Print("MockFix.ToAdmin", msg)
	if msg.IsMsgTypeOf("A") {
		msg.Body.SetString(fix.Tag(20000), m.ApiKey)
		msg.Body.SetString(fix.Tag(20001), m.ApiSecret)
		msg.Body.SetString(fix.Tag(20002), m.BfxUserID)
	}
	return
}

func (m *MockFIX) FromAdmin(msg *fix.Message, sID fix.SessionID) fix.MessageRejectError {
	log.Print("MockFix.FromAdmin", msg)
	return nil
}

func (m *MockFIX) ToApp(msg *fix.Message, sID fix.SessionID) error {
	log.Print("MockFix.ToApp", msg)
	s := m.Sessions[sID.String()]
	s.Sent = append(s.Sent, msg)
	return nil
}

func (m *MockFIX) FromApp(msg *fix.Message, sID fix.SessionID) fix.MessageRejectError {
	log.Print("MockFix.FromApp", msg)
	s := m.Sessions[sID.String()]
	s.m.Lock()
	defer s.m.Unlock()
	s.Received = append(s.Received, msg)
	return nil
}

func (m *MockFIX) OnCreate(sessionID fix.SessionID) {
	log.Print("MockFix.OnCreate ", sessionID)
	session := &Session{
		ID:       sessionID,
		LoggedOn: false,
		Received: make([]*fix.Message, 0),
		Sent:     make([]*fix.Message, 0),
	}
	m.last = session
	m.Sessions[sessionID.String()] = session
	// create counterparty session to remove after test
	dupe := fix.SessionID{
		BeginString:      sessionID.BeginString,
		TargetCompID:     sessionID.SenderCompID,
		TargetSubID:      sessionID.SenderSubID,
		TargetLocationID: sessionID.SenderLocationID,
		SenderCompID:     sessionID.TargetCompID,
		SenderSubID:      sessionID.TargetSubID,
		SenderLocationID: sessionID.TargetLocationID,
		Qualifier:        sessionID.Qualifier,
	}
	m.Sessions[dupe.String()] = &Session{
		ID: dupe,
	}
	return
}
