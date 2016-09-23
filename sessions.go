package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
)

type ConnManager struct {
	createChannel chan createConnection
	queryChannel  chan queryConnection
	closeChannel  chan string
}

type createConnection struct {
	reply chan string
}

type queryConnection struct {
	sessionId string
	reply     chan net.Conn
}

func NewConnManager() ConnManager {
	connections := ConnManager{
		createChannel: make(chan createConnection),
		queryChannel:  make(chan queryConnection),
		closeChannel:  make(chan string),
	}

	go connections.run()

	return connections
}

func (self ConnManager) CloseConnection(sessionId string) {
	self.closeChannel <- sessionId
}

func (self ConnManager) CreateConnection() (string, bool) {
	reply := make(chan string)
	self.createChannel <- createConnection{reply}
	sessionId, ok := <-reply
	return sessionId, ok
}

func (self ConnManager) FindConnection(sessionId string) (net.Conn, bool) {
	reply := make(chan net.Conn)
	self.queryChannel <- queryConnection{sessionId, reply}
	conn, ok := <-reply
	return conn, ok
}

func (self ConnManager) run() {
	connections := make(map[string]net.Conn)

	// TODO: We need to add at timer and check for stale connections

	for {
		select {
		case msg := <-self.createChannel:
			if conn, err := net.Dial("tcp", "localhost:22"); err != nil {
				log.Printf("Error creating ssh connection: %s", err)
				close(msg.reply)
			} else {
				if sessionId, err := generateSessionId(); err != nil {
					log.Printf("Error generating session id: %s", err)
					close(msg.reply)
				} else {
					// TODO: check that sessionId doesn't exist
					connections[sessionId] = conn
					msg.reply <- sessionId
				}
			}
		case msg := <-self.queryChannel:
			if conn, ok := connections[msg.sessionId]; ok {
				msg.reply <- conn
			} else {
				close(msg.reply)
			}
		case msg := <-self.closeChannel:
			if conn, ok := connections[msg]; ok {
				conn.Close()
				delete(connections, msg)
			}
		}
	}
}

func generateSessionId() (string, error) {
	buffer := make([]byte, 18)
	if _, err := rand.Read(buffer); err != nil {
		fmt.Println("Error creating random session id: ", err)
		return "", err
	}

	return base64.URLEncoding.EncodeToString(buffer), nil
}
