package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/tylertreat/bench"
)

var (
	dest        string
	url         string
	httpAddr    string
	origin      string
	rate        uint64
	connections uint64
	statsFreq   time.Duration
	dur         time.Duration

	launch = make(chan struct{})
	state  = "initializing"
)

func init() {
	flag.StringVar(&dest, "dest", "result.txt", "Filename to write the results to")
	flag.StringVar(&url, "url", "", "URL of websocket server")
	flag.StringVar(&httpAddr, "http-addr", ":80", "Interface and port to use for web server")
	flag.StringVar(&origin, "origin", "", "Origin host of websocket client")
	flag.Uint64Var(&rate, "rate", 0, "Number of requests to issue per second. Set to zero for full throttle")
	flag.Uint64Var(&connections, "conn", 1000, "Number of connections to create")
	flag.DurationVar(&dur, "duration", 30*time.Second, "How long to run the benchmark for")
	flag.DurationVar(&statsFreq, "stats-freq", 2*time.Second, "Frequency to update the stats")
	flag.Parse()
}

func main() {
	go server(httpAddr)
	r := &WebsocketRequesterFactory{
		URL:    url,
		Origin: origin,
	}

	<-launch
	fmt.Println("Initialising the connections to", url)
	fmt.Println("  Request rate:", rate)
	fmt.Println("  Number of connections:", connections)
	fmt.Println("  Benchmark duration:", dur)

	benchmark := bench.NewBenchmark(r, rate, connections, dur)
	summary, err := benchmark.Run()
	if err != nil {
		log.Println("Error:", err)
		return
	}

	fmt.Println(summary)
	summary.GenerateLatencyDistribution(bench.Logarithmic, dest)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

func server(addr string) {
	r := mux.NewRouter()
	r.HandleFunc("/socket", socketHandler)

	resOK := func(res http.ResponseWriter) {
		res.Header().Add("Access-Control-Allow-Origin", "*")
		res.WriteHeader(200)
	}

	r.HandleFunc("/launch", http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		close(launch)
		state = "Creating connections"
		resOK(res)
	}))

	r.HandleFunc("/begin", http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		close(release)
		state = "Starting Traffic"
		resOK(res)
	}))

	r.HandleFunc("/end", http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		close(end)
		state = "Retracting connections"
		resOK(res)
	}))

	http.ListenAndServe(httpAddr, r)
}

func socketHandler(w http.ResponseWriter, r *http.Request) {
	ret := make(chan struct{})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Failed to upgrade to websocket:", err)
		return
	}

	go ping(ws, ret)
	go pumpStats(ws, ret)

	cleanup(ret, ws)
}

func cleanup(r chan struct{}, ws *websocket.Conn) {
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			break
		}
	}

	close(r)
	ws.Close()
}

func pumpStats(ws *websocket.Conn, end chan struct{}) {
	st := &stats{}

	timer := time.NewTicker(statsFreq)
	for {
		select {
		case <-timer.C:
			s, err := getStats(st)
			if err != nil {
				log.Println("Failed to gather stats:", err)
				break
			}

			err = ws.WriteMessage(websocket.TextMessage, s)
			if err != nil {
				log.Println("Failed to write to socket:", err)
				break
			}

		case <-end:
			return
		}
	}
}

type stats struct {
	State              string `json:"state"`
	Standby            uint64 `json:"standby"`
	ActiveConnections  uint64 `json:"active_connections"`
	FailedConnections  uint64 `json:"failed_connections"`
	FailedRequests     uint64 `json:"failed_requests"`
	SuccessfulRequests uint64 `json:"successful_requests"`
	GracefulClose      uint64 `json:"graceful_close"`
	ErrorClose         uint64 `json:"error_close"`
}

func getStats(st *stats) ([]byte, error) {
	st.State = state

	sbl.RLock()
	st.Standby = standingBy
	sbl.RUnlock()

	boot.RLock()
	st.ActiveConnections = activeConn
	st.FailedConnections = failedConn
	st.GracefulClose = gracefulCompl
	st.ErrorClose = failedCompl
	boot.RUnlock()

	attempt.RLock()
	st.FailedRequests = failedReq
	st.SuccessfulRequests = succReq
	attempt.RUnlock()

	return json.Marshal(st)
}

func ping(ws *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait))
			if err != nil {
				log.Println("ping:", err)
			}
		case <-done:
			return
		}
	}
}
