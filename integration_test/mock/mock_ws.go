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
	ID     int
	parent *Ws
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

type tx struct {
	Msg      []byte
	ClientID int
}

//Ws is a mocked websocket instance
type Ws struct {
	clients  map[*client]bool
	listener net.Listener
	port     int

	register     chan *client
	unregister   chan *client
	broadcast    chan []byte
	send         chan *tx
	totalClients int
}

//WaitForClientCount waits for a specified number of clients to connect
func (s *Ws) WaitForClientCount(count int) error {
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

//TotalClientCount returns the total clients connected
func (s *Ws) TotalClientCount() int {
	return s.totalClients
}

//NewMockWs creates a new mocked websocket
func NewMockWs(port int) *Ws {
	return &Ws{
		port:       port,
		clients:    make(map[*client]bool),
		register:   make(chan *client),
		unregister: make(chan *client),
		broadcast:  make(chan []byte),
		send:       make(chan *tx),
	}
}

// Broadcast sends a message to all connected clients.
func (s *Ws) Broadcast(msg string) {
	s.broadcast <- []byte(msg)
}

// Send emits a message to the client
func (s *Ws) Send(clientID int, msg string) {
	s.send <- &tx{ClientID: clientID, Msg: []byte(msg)}
}

// ReceivedCount starts indexing clients at position 0.
func (s *Ws) ReceivedCount(clientNum int) int {
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
func (s *Ws) Received(clientNum int, msgNum int) (string, error) {
	var client *client
	i := 0
	for c := range s.clients {
		if i == clientNum {
			client = c
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

//DumpRecv dumps all received messages from the websocket
func (s *Ws) DumpRecv() {
	i := 0
	for c := range s.clients {
		log.Printf("received for client %d:\n", c.ID)
		for j, m := range c.received {
			log.Printf("%2d: %s", j, m)
		}
		i++
	}
}

//WaitForMessage waits until a message has come in for the specified client
func (s *Ws) WaitForMessage(clientNum int, msgNum int) (string, error) {
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
	s.DumpRecv()
	return "", err
}

func (s *Ws) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.serveWs(w, r)
}

//KillConnections closes all active connections
func (s *Ws) KillConnections() (err error) {
	for c := range s.clients {
		log.Printf("killing connection for client %d:\n", c.ID)
		if err = c.Conn.Close(); err != nil {
			return
		}
	}
	return
}

//Stop ceases listening to http
func (s *Ws) Stop() error {
	return s.listener.Close()
}

//Start begins listening to http
func (s *Ws) Start() error {
	go s.loop()
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}
	s.listener = l
	go http.Serve(s.listener, s)
	return nil
}

func (s *Ws) serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print(err)
		return
	}
	client := &client{ID: s.totalClients, parent: s, Conn: conn, send: make(chan []byte, 256), received: make([]string, 0)}
	s.totalClients++
	go client.writePump()
	go client.readPump()
	s.clients[client] = true
}

func (s *Ws) loop() {
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
					log.Printf("failed to send message to client %d", client.ID)
					close(client.send)
					delete(s.clients, client)
				}
			}
		case tx := <-s.send:
			for client := range s.clients {
				if client.ID != tx.ClientID {
					continue
				}
				select {
				case client.send <- tx.Msg:
				default:
					log.Printf("failed to send message to client %d", client.ID)
					close(client.send)
					delete(s.clients, client)
				}
			}
		}
	}
}
