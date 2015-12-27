package main

import (
	"log"
	"sync"

	"github.com/tylertreat/bench"
	"golang.org/x/net/websocket"
)

var (
	boot          sync.RWMutex
	activeConn    uint64
	failedConn    uint64
	gracefulCompl uint64
	failedCompl   uint64

	sbl        sync.RWMutex
	standingBy uint64

	attempt   sync.RWMutex
	failedReq uint64
	succReq   uint64

	release = make(chan struct{})
	end     = make(chan struct{})
)

// WebRequesterFactory implements RequesterFactory by creating a Requester
// which makes GET requests to the provided URL.
type WebsocketRequesterFactory struct {
	URL    string
	Origin string
}

// GetRequester returns a new Requester, called for each Benchmark connection.
func (w *WebsocketRequesterFactory) GetRequester(uint64) bench.Requester {
	return &websocketRequester{
		url:    w.URL,
		origin: w.Origin,
	}
}

// webRequester implements Requester by making a GET request to the provided
// URL.
type websocketRequester struct {
	url    string
	origin string
	data   []byte
	conn   *websocket.Conn
}

// Setup prepares the Requester for benchmarking.
func (w *websocketRequester) Setup() error {
	var err error
	w.conn, err = websocket.Dial(w.url, "", w.origin)

	boot.Lock()
	if err != nil {
		failedConn++
	} else {
		activeConn++
	}
	boot.Unlock()

	sbl.Lock()
	standingBy++
	sbl.Unlock()

	return err
}

// Request performs a synchronous request to the system under test.
func (w *websocketRequester) Request() error {
	var err error

	<-release
	sbl.Lock()
	standingBy--
	sbl.Unlock()

	_, err = w.conn.Write(w.data)

	attempt.Lock()
	if err != nil {
		failedReq++
		log.Println("failed to send message on socket:", err)
	} else {
		succReq++
	}
	attempt.Unlock()

	return err
}

// Teardown is called upon benchmark completion.
func (w *websocketRequester) Teardown() error {
	err := w.conn.Close()

	boot.Lock()
	if err != nil {
		log.Println("failed to shutdown:", err)
		failedCompl++
	} else {
		gracefulCompl++
	}
	activeConn--
	boot.Unlock()
	return err
}
