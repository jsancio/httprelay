package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
)

// TODO: we need to make sure that we remove connections after being inactive

func main() {
	connections := NewConnManager()

	http.HandleFunc("/cookie", cookie)
	http.Handle("/proxy", proxyHandler{connections})
	http.Handle("/read", readHandler{connections})
	http.Handle("/write", writeHandler{connections})

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
	connections ConnManager
}

func (self proxyHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	if sessionId, ok := self.connections.CreateConnection(); !ok {
		writer.WriteHeader(500)
		writer.Write([]byte{})
	} else {
		fmt.Fprint(writer, sessionId)
	}
}

type readHandler struct {
	connections ConnManager
}

func (self readHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	sessionId := request.URL.Query()["sid"][0]
	conn, ok := self.connections.FindConnection(sessionId)
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

		self.connections.CloseConnection(sessionId)

		writer.WriteHeader(410)
		if _, err := writer.Write([]byte{}); err != nil {
			log.Printf("Error writing zero bytes: %s", err)
		}
	} else {
		if _, err := writer.Write(
			[]byte(base64.URLEncoding.EncodeToString(buffer[:bytes]))); err != nil {

			log.Printf("Error writing ssh bytes: %s", err)

			self.connections.CloseConnection(sessionId)
		}
	}
}

type writeHandler struct {
	connections ConnManager
}

func (self writeHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	sessionId := request.URL.Query()["sid"][0]
	conn, ok := self.connections.FindConnection(sessionId)
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

		self.connections.CloseConnection(sessionId)

		writer.WriteHeader(410)
	} else {
		_, err = conn.Write(buffer)
		if err != nil {
			log.Printf("Error writing to ssh socket: %s", err)

			self.connections.CloseConnection(sessionId)

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

func padData(data string) string {
	missingPadding := (4 - len(data)%4) % 4
	for missingPadding > 0 {
		data += "="
		missingPadding -= 1
	}

	return data
}
