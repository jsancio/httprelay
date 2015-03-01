package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
)

// TODO: we need to make sure that we remove connections after a timeouto of inactive

func main() {
	createChannel := make(chan createConnection)
	queryChannel := make(chan queryConnection)
	go sessionManager(createChannel, queryChannel)

	http.HandleFunc("/cookie", cookie)
	http.Handle("/proxy", proxyHandler{createChannel})
	http.Handle("/read", readHandler{queryChannel})
	http.Handle("/write", writeHandler{queryChannel})

	log.Fatal(http.ListenAndServe(":5000", nil))
}

func cookie(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	ext := request.URL.Query().Get("ext")
	path := request.URL.Query().Get("path")

	location := fmt.Sprintf(
		"chrome-extension://%s/%s#anonymous@192.168.56.101:5000",
		ext,
		path)

	writer.Header().Set("Location", location)
	writer.WriteHeader(302)
	_, err := writer.Write([]byte{}) // TODO: I think this is an allocation
	if err != nil {
		log.Printf("Error writing zero bytes: %s", err)
	}
}

type proxyHandler struct {
	createChannel chan createConnection
}

func (self proxyHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	reply := make(chan string)
	self.createChannel <- createConnection{reply}
	if sessionId, ok := <-reply; !ok {
		writer.WriteHeader(500)
		writer.Write([]byte{})
	} else {
		fmt.Fprint(writer, sessionId)
	}
}

type readHandler struct {
	queryChannel chan queryConnection
}

func (self readHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	conn, ok := findConnection(request.URL.Query()["sid"][0], self.queryChannel)
	if !ok {
		writer.WriteHeader(410)
		if _, err := writer.Write([]byte{}); err != nil {
			log.Printf("Error writing zero bytes: %s", err)
		}

		return
	}

	var buffer [1024]byte

	if bytes, err := conn.Read(buffer[:]); err != nil {
		log.Printf("Error reading from ssh socket: %s", err)

		// TODO: send close message to session manager
		conn.Close()

		writer.WriteHeader(410)
		if _, err := writer.Write([]byte{}); err != nil {
			log.Printf("Error writing zero bytes: %s", err)
		}
	} else {
		if _, err := writer.Write(
			[]byte(base64.URLEncoding.EncodeToString(buffer[:bytes]))); err != nil {

			log.Printf("Error writing ssh bytes: %s", err)

			// TODO: send close message to session manager
			conn.Close()
		}
	}
}

type writeHandler struct {
	queryChannel chan queryConnection
}

func (self writeHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	conn, ok := findConnection(request.URL.Query()["sid"][0], self.queryChannel)
	if !ok {
		writer.WriteHeader(410)
		if _, err := writer.Write([]byte{}); err != nil {
			log.Printf("Error writing zero bytes: %s", err)
		}

		return
	}

	data := padData(request.URL.Query().Get("data"))

	fmt.Printf("Decoding %d bytes: %s", len(data), data)
	if buffer, err := base64.URLEncoding.DecodeString(data); err != nil {
		log.Printf("Error decoding data: %s", err)

		// TODO: send close message to session manager
		conn.Close()

		writer.WriteHeader(410)
	} else {
		_, err = conn.Write(buffer)
		if err != nil {
			log.Printf("Error writing to ssh socket: %s", err)

			// TODO: send close message to session manager
			conn.Close()

			writer.WriteHeader(410)
		} else {
			writer.WriteHeader(200)
		}
	}

	_, err := writer.Write([]byte{}) // TODO: I think this is an allocation
	if err != nil {
		log.Printf("Error writing zero bytes: %s", err)
	}
}

func setHeaders(writer http.ResponseWriter, origin string) {
	writer.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	writer.Header().Set("Pragma", "no-cache")
	writer.Header().Set("Access-Control-Allow-Credentials", "true")
	writer.Header().Set("Access-Control-Allow-Origin", origin)
}

func generateSessionId() (string, error) {
	buffer := make([]byte, 18)
	if _, err := rand.Read(buffer); err != nil {
		fmt.Println("Error creating random session id: ", err)
		return "", err
	}

	return base64.URLEncoding.EncodeToString(buffer), nil
}

func padData(data string) string {
	missingPadding := (4 - len(data)%4) % 4
	for missingPadding > 0 {
		data += "="
		missingPadding -= 1
	}

	return data
}

type createConnection struct {
	reply chan string
}

type queryConnection struct {
	sessionId string
	reply     chan net.Conn
}

func sessionManager(create chan createConnection, query chan queryConnection) {
	connections := make(map[string]net.Conn)

	// TODO: We need to add at timer and check for stale connections

	for {
		select {
		case msg := <-create:
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
		case msg := <-query:
			if conn, ok := connections[msg.sessionId]; ok {
				msg.reply <- conn
			} else {
				close(msg.reply)
			}
		}
	}
}

func findConnection(sessionId string, queryChannel chan queryConnection) (net.Conn, bool) {
	reply := make(chan net.Conn)
	queryChannel <- queryConnection{sessionId, reply}
	conn, ok := <-reply
	return conn, ok
}
