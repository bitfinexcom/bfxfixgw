package mock

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	fix "github.com/quickfixgo/quickfix"
)

// Session corresponds to key = SessionID
type Session struct {
	ID       fix.SessionID
	LoggedOn bool
	Received map[int]string
	Sent     map[int]string
	m        sync.Mutex
	wg       sync.WaitGroup
}

// Send emits a FIX message
func (s *Session) Send(msg fix.Messagable) {
	fix.SendToTarget(msg, s.ID)
}

// TestFixClient is a FIX client for testing purposes
type TestFixClient struct {
	initiator *fix.Initiator
	settings  *fix.Settings
	router    *fix.MessageRouter

	Sessions map[string]*Session
	last     *Session

	sendOnLogon []fix.Messagable

	OmitLogMessages bool
	MessageHandler

	APIKey, APISecret, BfxUserID, name string
	CancelOnDisconnect                 bool
}

// Handle processes a FIX message
func (m *TestFixClient) Handle(msg *fix.Message) {
	// no-op
}

//MessageHandler is a generic FIX message handler
type MessageHandler interface {
	Handle(msg *fix.Message)
}

//SetMessageHandler assigns a message handler
func (m *TestFixClient) SetMessageHandler(handler MessageHandler) {
	m.MessageHandler = handler
}

//Send emits a FIX message
func (m *TestFixClient) Send(msg fix.Messagable) error {
	err := fix.SendToTarget(msg, m.last.ID)
	soh := byte(0x1)
	str := strings.Replace(msg.ToMessage().String(), string([]byte{soh}), "|", -1)
	log.Printf("[-> %s]: %s", m.last.ID.String(), str)
	return err
}

//WaitForSession tries to find a FIX session
func (m *TestFixClient) WaitForSession(sid string) error {
	found := false
	for i := 0; i < 20; i++ {
		if session, ok := m.Sessions[sid]; ok {
			if session.LoggedOn {
				log.Printf("found session %s", sid)
				return nil
			}
			found = true
		}
		time.Sleep(time.Millisecond * 250)
	}
	for _, s := range m.Sessions {
		log.Printf("test fix client session: %s", s.ID.String())
	}
	if found {
		return fmt.Errorf("session found but not logged on: %s", sid)
	}
	return fmt.Errorf("could not find session: %s", sid)
}

//LastSession returns the last FIX session
func (m *TestFixClient) LastSession() *Session {
	return m.last
}

//SendFIX emits a FIX message
func (m *TestFixClient) SendFIX(msg fix.Messagable) {
	if len(m.Sessions) > 0 {
		for _, s := range m.Sessions {
			s.Send(msg)
			return
		}
	}
}

//NewTestFixClient creates a new testing FIX client
func NewTestFixClient(settings *fix.Settings, msgStore fix.MessageStoreFactory, name string) (*TestFixClient, error) {
	f := &TestFixClient{
		router:   fix.NewMessageRouter(),
		settings: settings,
		Sessions: make(map[string]*Session),
		name:     name,
	}
	f.MessageHandler = f
	/*logFactory, err := fix.NewFileLogFactory(settings)
	if err != nil {
		log.Print("could not create file log factory")
		return nil, err
	}*/
	logFactory := fix.NewScreenLogFactory()
	initiator, err := fix.NewInitiator(f, msgStore, settings, logFactory)
	if err != nil {
		log.Printf("could not create acceptor: %s", err.Error())
		return nil, err
	}
	f.initiator = initiator
	return f, nil
}

//ReceivedCount finds the messages received per session
func (m *TestFixClient) ReceivedCount(sessionID string) int {
	if s, ok := m.Sessions[sessionID]; ok {
		s.m.Lock()
		defer s.m.Unlock()
		return len(s.Received)
	}
	return 0
}

//WaitForMessage waits for a message at the specified sequence number
func (m *TestFixClient) WaitForMessage(sessionID string, seqnum int) (string, error) {
	return m.WaitForMessageWithWait(sessionID, seqnum, time.Second*4)
}

//WaitForMessageWithWait waits for a session message with the given timeout
func (m *TestFixClient) WaitForMessageWithWait(sessionID string, seqnum int, wait time.Duration) (string, error) {
	if s, ok := m.Sessions[sessionID]; ok {
		loops := 25
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

//DumpRcvFIX prints out the messages for a given session
func (m *TestFixClient) DumpRcvFIX(sid string) {
	log.Printf("Messages for %s:\n", sid)
	if s, ok := m.Sessions[sid]; ok {
		for i, m := range s.Received {
			log.Printf("%2d: %s", i, m)
		}
	}
}

//Start begins the initiator
func (m *TestFixClient) Start() error {
	return m.initiator.Start()
}

//Stop stops the initiator
func (m *TestFixClient) Stop() {
	m.initiator.Stop()
	for _, s := range m.Sessions {
		err := fix.UnregisterSession(s.ID)
		if err != nil {
			log.Printf("[FIX %s] MockFix.Stop: could not remove session %s: %s", m.name, s.ID.String(), err.Error())
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
			log.Printf("[FIX %s] MockFix.Stop: could not remove session %s: %s", m.name, counterparty.String(), err.Error())
		}
	}
}

//OnLogout logs out of the specified session
func (m *TestFixClient) OnLogout(sessionID fix.SessionID) {
	log.Printf("[FIX %s] MockFix.OnLogout: %s", m.name, sessionID)
	m.Sessions[sessionID.String()].LoggedOn = false
	return
}

//SendOnLogon sets the logon messages
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

//OnLogon handles a session ID logon
func (m *TestFixClient) OnLogon(sessionID fix.SessionID) {
	log.Printf("[FIX %s] MockFix.OnLogon: %s", m.name, sessionID)
	m.Sessions[sessionID.String()].LoggedOn = true
	m.onLogon(sessionID)
	return
}

func fixString(msg fix.Messagable) string {
	return strings.Replace(msg.ToMessage().String(), string([]byte{0x1}), "|", -1)
}

// ToAdmin handles outgoing admin
func (m *TestFixClient) ToAdmin(msg *fix.Message, sessionID fix.SessionID) {
	msgType, err := msg.MsgType()
	if err != nil {
		return
	}
	if "A" == msgType {
		msg.Body.SetString(fix.Tag(20000), m.APIKey)
		msg.Body.SetString(fix.Tag(20001), m.APISecret)
		msg.Body.SetString(fix.Tag(20002), m.BfxUserID)
		if m.CancelOnDisconnect {
			msg.Body.SetBool(fix.Tag(8013), true)
		}
	}
	log.Printf("[FIX %s] MockFix.ToAdmin (outgoing): %s", m.name, fixString(msg))
	return
}

// FromAdmin handles incoming admin
func (m *TestFixClient) FromAdmin(msg *fix.Message, sID fix.SessionID) fix.MessageRejectError {
	log.Printf("[FIX %s] MockFix.FromAdmin (incoming): %s", m.name, fixString(msg))
	s := m.Sessions[sID.String()]
	s.m.Lock()
	defer s.m.Unlock()
	seq, err := msg.Header.GetInt(34)
	if err != nil {
		return err
	}
	s.Received[seq] = strings.Replace(msg.String(), string(0x1), "|", -1)
	return nil
}

// ToApp handles outgoing app
func (m *TestFixClient) ToApp(msg *fix.Message, sID fix.SessionID) error {
	if !m.OmitLogMessages {
		log.Printf("[FIX %s] MockFix.ToApp (outgoing): %s", m.name, fixString(msg))
	}
	s := m.Sessions[sID.String()]
	seq, err := msg.Header.GetInt(34)
	if err != nil {
		return err
	}
	s.Sent[seq] = strings.Replace(msg.String(), string(0x1), "|", -1)
	return nil
}

// FromApp handles incoming app
func (m *TestFixClient) FromApp(msg *fix.Message, sID fix.SessionID) fix.MessageRejectError {
	if !m.OmitLogMessages {
		log.Printf("[FIX %s] MockFix.FromApp (incoming): %s", m.name, fixString(msg))
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

//OnCreate handles session creation
func (m *TestFixClient) OnCreate(sessionID fix.SessionID) {
	log.Printf("[FIX %s] MockFix.OnCreate: %s", m.name, sessionID)
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
