package main

import (
	"flag"
	"log"

	"go-get-request/events"
	"go-get-request/gui"
	"go-get-request/mock"
	"go-get-request/store"
)

func main() {
	port := flag.Int("port", 8080, "GUI port")
	flag.Parse()

	s := store.New()
	b := events.NewBus()
	m := mock.New(s, b)
	g := gui.New(*port, s, b, m)

	log.Fatal(g.Start())
}
