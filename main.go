package main

import (
	"encoding/hex"
	"fmt"
	"net"

	"github.com/lithdew/monte"
	"gitlab.com/NebulousLabs/go-upnp"
)

func main() {
	check := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	// connect UPnP to router
	fmt.Println("Discovering router")
	d, err := upnp.Discover()
	check(err)
	// discover external IP
	fmt.Println("Checking external IP")
	ip, err := d.ExternalIP()
	check(err)
	fmt.Println("Your external IP is:", ip)
	// check if the port is forwarded
	fmt.Println("Checking if port is already forwarded.")
	isFwd, err := d.IsForwardedTCP(52386)
	check(err)
	// forward a port
	if !isFwd {
		fmt.Println("Requesting port forward.")
		err = d.Forward(52386, "haukened/splice")
		check(err)
		fmt.Println("Forwarded port 52386")
	} else {
		fmt.Println("Port 52386 is already forwarded")
	}
	// test monte
	go func() {
		conn, err := net.Dial("tcp", ":52386")

		var sess monte.Session
		check(sess.DoClient(conn))

		fmt.Println(hex.EncodeToString(sess.SharedKey()))

		sc := monte.NewSessionConn(sess.Suite(), conn)

		for i := 0; i < 100; i++ {
			_, err = sc.Write([]byte(fmt.Sprintf("[%d] Hello from Go!", i)))
			check(err)
			check(sc.Flush())
		}

	}()

	ln, err := net.Listen("tcp", ":52386")
	check(err)
	defer ln.Close()

	conn, err := ln.Accept()
	check(err)
	defer conn.Close()

	var sess monte.Session
	check(sess.DoServer(conn))

	fmt.Println(hex.EncodeToString(sess.SharedKey()))

	sc := monte.NewSessionConn(sess.Suite(), conn)

	buf := make([]byte, 1024)

	for i := 0; i < 100; i++ {
		n, err := sc.Read(buf)
		check(err)
		fmt.Println("Decrypted:", string(buf[:n]))
	}

	// unforward the port
	err = d.Clear(52386)
	check(err)
	fmt.Println("Unforwarded port. Bye.")
}
