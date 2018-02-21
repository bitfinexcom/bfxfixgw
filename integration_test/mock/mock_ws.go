package mock

import (
	"bytes"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type client struct {
	parent *MockWs
	*websocket.Conn
	send     chan []byte
	received []string
	lock     sync.Mutex
}

func (c *client) writePump() {
	for msg := range c.send {
		err := c.Conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			log.Printf("could not send message to client: %s", err.Error())
			continue
		}
	}
}

func (c *client) readPump() {
	defer func() {
		c.parent.unregister <- c
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(512)
	c.Conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(10 * time.Second)); return nil })
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message, []byte("\n"), []byte(" "), -1))
		if string(message) == "" {
			log.Printf("got empty message!")
		}
		c.lock.Lock()
		c.received = append(c.received, string(message))
		c.lock.Unlock()
	}
}

type MockWs struct {
	clients  map[*client]bool
	listener net.Listener
	port     int

	register     chan *client
	unregister   chan *client
	broadcast    chan []byte
	totalClients int
}

func (s *MockWs) WaitForClientCount(count int) error {
	loops := 16
	delay := time.Millisecond * 250
	for i := 0; i < loops; i++ {
		if s.totalClients == count {
			return nil
		}
		time.Sleep(delay)
	}
	return fmt.Errorf("client peer #%d did not connect", count)
}

func (s *MockWs) TotalClientCount() int {
	return s.totalClients
}

func NewMockWs(port int) *MockWs {
	return &MockWs{
		port:       port,
		clients:    make(map[*client]bool),
		register:   make(chan *client),
		unregister: make(chan *client),
		broadcast:  make(chan []byte),
	}
}

// Broadcast sends a message to all connected clients.
func (s *MockWs) Broadcast(msg string) {
	s.broadcast <- []byte(msg)
}

// ReceivedCount starts indexing clients at position 0.
func (s *MockWs) ReceivedCount(clientNum int) int {
	i := 0
	for client := range s.clients {
		if i == clientNum {
			client.lock.Lock()
			defer client.lock.Unlock()
			return len(client.received)
		}
		i++
	}
	return 0
}

// Received starts indexing clients and message positions at position 0.
func (s *MockWs) Received(clientNum int, msgNum int) (string, error) {
	var client *client
	i := 0
	for client = range s.clients {
		if i == clientNum {
			break
		}
		i++
	}
	if client != nil {
		client.lock.Lock()
		defer client.lock.Unlock()
		if len(client.received) > msgNum {
			return string(client.received[msgNum]), nil
		}
		return "", fmt.Errorf("could not find message index %d, %d messages exist", msgNum, len(client.received))
	}
	return "", fmt.Errorf("could not find client %d", clientNum)
}

func (s *MockWs) WaitForMessage(clientNum int, msgNum int) (string, error) {
	loops := 16
	delay := time.Millisecond * 250
	var msg string
	var err error
	for i := 0; i < loops; i++ {
		msg, err = s.Received(clientNum, msgNum)
		if err != nil {
			time.Sleep(delay)
		} else {
			return msg, nil
		}
	}
	return "", err
}

func (s *MockWs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.serveWs(w, r)
}

func (s *MockWs) Stop() {
	s.listener.Close() // stop listening to http
}

func (s *MockWs) Start() error {
	go s.loop()
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}
	s.listener = l
	go http.Serve(s.listener, s)
	return nil
}

func (s *MockWs) serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print(err)
		return
	}
	s.totalClients++
	client := &client{parent: s, Conn: conn, send: make(chan []byte, 256), received: make([]string, 0)}
	go client.writePump()
	go client.readPump()
	s.clients[client] = true
}

func (s *MockWs) loop() {
	for {
		select {
		case client := <-s.register:
			s.clients[client] = true
		case client := <-s.unregister:
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				close(client.send)
			}
		case msg := <-s.broadcast:
			for client := range s.clients {
				select {
				case client.send <- msg:
				default: // send failure
					close(client.send)
					delete(s.clients, client)
				}
			}
		}
	}
}
