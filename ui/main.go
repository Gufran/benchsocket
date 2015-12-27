package main

import (
	"flag"
	"log"
	"net/http"
	"os"
)

var addr string

func main() {
	flag.StringVar(&addr, "addr", ":80", "Interface and port to mount the server")
	flag.Parse()

	uiDir, err := os.Getwd()
	if err != nil {
		log.Fatalln("cannot get current directory:", err)
	}

	err = http.ListenAndServe(addr, http.FileServer(http.Dir(uiDir)))
	if err != nil {
		log.Println("Servere crashed:", err)
	}
}
