package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"time"

	"github.com/lithdew/flatend"
	"github.com/urfave/cli/v2"
	"gitlab.com/NebulousLabs/go-upnp"
	"go.uber.org/zap"
)

// TODO remove designation and update at build time
var version = "v0.0.0"

func check(err error) bool {
	if err != nil {
		return true
	}
	return false
}

func main() {
	if err := run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	app := cli.App{
		Name:                   "Splice",
		HelpName:               "Splice",
		Usage:                  "Brought to you by the hug-free vikings",
		UseShortOptionHandling: true,
		Writer:                 stdout,
		ErrWriter:              stderr,
		Version:                version,

		Flags: []cli.Flag{
			&cli.UintFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Manually specify a port for this node to listen on.",
				Value:   52386,
			},
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Usage:   "Enable debugging mode.",
				Value:   false,
			},
			&cli.BoolFlag{
				Name:    "disable-upnp",
				Aliases: []string{"noupnp"},
				Usage:   "Disables UPnP port forwarding",
				Value:   false,
			},
			&cli.StringFlag{
				Name:    "bootstrap-peer",
				Aliases: []string{"peer"},
				Usage:   "Connect to a known host:port to perform boostrapping",
			},
			&cli.StringFlag{
				Name:  "public-address",
				Usage: "Your public IP address, required if UPnP discovery is disabled",
			},
			/*&cli.PathFlag{
				Name:    "load-private-key",
				Aliases: []string{"l"},
				Usage:   "Specify a private key, otherwise a new one will be generated for you",
			},*/
		},
		Action: actStartNode,
	}

	if err := app.Run(args); err != nil {
		return err
	}

	return nil
}

func actStartNode(c *cli.Context) error {
	var (
		localPort = c.Uint("port")
		debug     = c.Bool("debug")
		//PrivateKeyPath = c.Path("load-private-key")
		disableUPnP = c.Bool("disable-upnp")
		peer        = c.String("bootstrap-peer")
		publicIP    = c.String("public-address")
	)

	// set up a logger
	var logger *zap.Logger
	var err error
	if debug {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if check(err) {
		fmt.Fprintf(os.Stderr, "Unable to initialize logger: %v\n", err)
		return err
	}
	defer logger.Sync()
	logger.Debug("Logging in debug mode.")

	// determine the listening port
	if localPort <= 0 || localPort > math.MaxUint16 {
		return fmt.Errorf("'%d' is an invalid port", localPort)
	}
	bindPort := uint16(localPort)

	// set up UPnP if required
	if !disableUPnP {
		err = forwardUpnpPort(bindPort)
		if check(err) {
			return err
		}
		logger.Sugar().Debugf("UPnP successfully forwarded port %d", bindPort)
		// get public IP address using UPnP
		publicIP, err = getPublicIPAddress()
		if check(err) {
			return err
		}
		logger.Sugar().Debugf("setting node public IP address to %s", publicIP)
	} else if publicIP == "" {
		return fmt.Errorf("public ip address is required if UPnP is disabled")
	}

	// set up the server node
	node := &flatend.Node{
		PublicAddr: fmt.Sprintf("%s:%d", publicIP, bindPort),
		BindAddrs:  []string{fmt.Sprintf(":%d", bindPort)},
		SecretKey:  flatend.GenerateSecretKey(),
		Services: map[string]flatend.Handler{
			"chat": func(ctx *flatend.Context) {
				buf, err := ioutil.ReadAll(ctx.Body)
				if check(err) {
					return
				}
				fmt.Printf("%s: %s\n", ctx.ID.Host.String(), string(buf))
			},
		},
	}
	defer node.Shutdown()
	err = node.Start(peer)

	// then process input and declare us as a chat provider
	br := bufio.NewReader(os.Stdin)
	for { // ever
		line, _, err := br.ReadLine()
		if check(err) {
			return err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		providers := node.ProvidersFor("chat")
		for _, provider := range providers {
			_, err := provider.Push([]string{"chat"}, nil, ioutil.NopCloser(bytes.NewReader(line)))
			if check(err) {
				logger.Sugar().Warnf("Unable to broadcast to %s: %s\n", provider.Addr(), err)
			}
		}
	}
}

func forwardUpnpPort(port uint16) error {
	// connect UPnP to router
	d, err := upnp.Discover()
	if check(err) {
		return fmt.Errorf("upnp is unable to discover your router: %v", err)
	}
	// check if port is already forwarded
	isFwd, err := d.IsForwardedTCP(52386)
	if check(err) {
		return fmt.Errorf("upnp is unable to check if port %d is already forwarded: %v", port, err)
	}
	if isFwd {
		return nil
	}
	// forward a port
	err = d.Forward(52386, "haukened/splice")
	if check(err) {
		return fmt.Errorf("upnp is unable to forward port %d: %v", port, err)
	}
	return nil
}

func clearUpnpPort(port uint16) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, err := upnp.DiscoverCtx(ctx)
	if check(err) {
		return err
	}
	err = d.Clear(port)
	if check(err) {
		return err
	}
	return nil
}

func getPublicIPAddress() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, err := upnp.DiscoverCtx(ctx)
	if check(err) {
		return "", err
	}
	return d.ExternalIP()
}
