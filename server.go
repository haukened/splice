package main

import (
	"net"
	"sync"

	"github.com/lithdew/kademlia"
	"github.com/lithdew/monte"
)

// Service holds the name of a Service, to be bound to a handler
type Service string

// Handler holds the pointer to a function, bound by a Service
type Handler func(ctx *monte.Context) error

// PublicAddress holds the dial information as a string in the format "host:port"
type PublicAddress string

// Server holds all information for listening and responding to requests
type Server struct {
	// Services holds service names and bound handlers in a map
	Services map[Service]Handler

	// PublicKey is the *kademlia.PublicKey of this server
	PublicKey *kademlia.PublicKey

	bindAddr []net.IP // the private addresses this node will bind. empty will bind 0.0.0.0 (all interfaces)
	port     uint16   // the port to bind.  By default this will be 52386

	clients     map[kademlia.PublicKey]*monte.Client // allows mapping a PublicKey to a Client
	clientsLock sync.Mutex                           // for safety!

	id        *kademlia.ID        // the ID of this Server node
	listener  []net.Listener      // this Server node's listener
	server    *monte.Server       // the instance of this server's monte implementation
	secretKey kademlia.PrivateKey // this Server's PrivateKey
	start     sync.Once           // handles starting of this server
	stop      sync.Once           // handles stopping of this server
	table     *kademlia.Table     // this Server's routing table
	tableLock sync.Mutex          // for safety!
	wg        sync.WaitGroup      // a waitgroup
}
