package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
)

func main() {
	http.HandleFunc("/cookie", cookie)
	http.HandleFunc("/proxy", proxy)
	http.HandleFunc("/read", read)
	http.HandleFunc("/write", write)

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

var conn net.Conn = nil

func proxy(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	localConn, err := net.Dial("tcp", "localhost:22")
	if err != nil {
		log.Printf("Error creating ssh connection: %s", err)
		writer.WriteHeader(500)
		fmt.Fprintf(writer, "Error creating ssh connection: %s", err)
	}

	conn = localConn

	fmt.Fprint(writer, "some_secret")
}

func read(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	if conn == nil {
		writer.WriteHeader(410)
		_, err := writer.Write([]byte{}) // TODO: I think this is an allocation
		if err != nil {
			log.Printf("Error writing zero bytes: %s", err)
		}
	}

	var buffer [1024]byte

	// Make sure that we call conn.SetDeadline(time.Now().Add(time.Duration(60)))
	bytes, err := conn.Read(buffer[:])
	if err != nil {
		log.Printf("Error reading from ssh socket: %s", err)
		conn = nil
		writer.WriteHeader(410)
		_, err := writer.Write([]byte{}) // TODO: I think this is an allocation
		if err != nil {
			log.Printf("Error writing zero bytes: %s", err)
		}
	} else {
		_, err := writer.Write([]byte(base64.URLEncoding.EncodeToString(buffer[:bytes])))
		if err != nil {
			log.Printf("Error writing ssh bytes: %s", err)
		}
	}
}

func write(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Handling Request: %s", request)

	setHeaders(writer, request.Header.Get("Origin"))

	if conn == nil {
		writer.WriteHeader(410)
		_, err := writer.Write([]byte{}) // TODO: I think this is an allocation
		if err != nil {
			log.Printf("Error writing zero bytes: %s", err)
		}
	}

	data := request.URL.Query().Get("data")

	missingPadding := (4 - len(data)%4) % 4
	for missingPadding > 0 {
		data += "="
		missingPadding -= 1
	}

	fmt.Printf("Decoding %d bytes: %s", len(data), data)
	buffer, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		log.Printf("Error decoding data: %s", err)
		conn = nil
		writer.WriteHeader(410)
	} else {
		_, err = conn.Write(buffer)
		if err != nil {
			log.Printf("Error writing to ssh socket: %s", err)
			conn = nil
			writer.WriteHeader(410)
		} else {
			writer.WriteHeader(200)
		}
	}

	_, err = writer.Write([]byte{}) // TODO: I think this is an allocation
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
