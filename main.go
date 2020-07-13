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
	"github.com/thibran/pubip"
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
		publicIP    string
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
	logger.Sugar().Debugf("Setting port to %d", bindPort)

	// set up UPnP if required
	if !disableUPnP {
		logger.Debug("Sending UPnP router discovery")
		router, err := discoverUpnpRouter()
		if check(err) {
			return err
		}
		logger.Sugar().Debugf("UPnP router discovered at %s", router)
		err = forwardUpnpPort(router, bindPort)
		if check(err) {
			return err
		}
		logger.Sugar().Debugf("UPnP successfully forwarded port %d", bindPort)
		// get public IP address using UPnP
		publicIP, err = getUpnpPublicAddress(router)
		if check(err) {
			return err
		}
		logger.Sugar().Debugf("setting node public IP address to %s", publicIP)
	} else if publicIP == "" {
		publicIP, err = getPublicIPAddress()
		logger.Sugar().Debugf("setting node public IP address to %s", publicIP)
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
				ident := ctx.ID.Pub.String()
				fmt.Printf("%s: %s\n", ident[len(ident)-6:], string(buf))
			},
		},
	}
	err = node.Start(peer)
	defer node.Shutdown()

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

func discoverUpnpRouter() (routerAddress string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, err := upnp.DiscoverCtx(ctx)
	if check(err) {
		return
	}
	routerAddress = d.Location()
	return
}

func forwardUpnpPort(routerAddress string, port uint16) error {
	// connect UPnP to router
	d, err := upnp.Load(routerAddress)
	if check(err) {
		return fmt.Errorf("upnp is unable to connect to your router: %v", err)
	}
	// check if port is already forwarded
	isFwd, err := d.IsForwardedTCP(port)
	if check(err) {
		return fmt.Errorf("upnp is unable to check if port %d is already forwarded: %v", port, err)
	}
	if isFwd {
		return nil
	}
	// forward a port
	err = d.Forward(port, "haukened/splice")
	if check(err) {
		return fmt.Errorf("upnp is unable to forward port %d: %v", port, err)
	}
	return nil
}

func clearUpnpPort(routerAddress string, port uint16) error {
	// connect UPnP to router
	d, err := upnp.Load(routerAddress)
	if check(err) {
		return fmt.Errorf("upnp is unable to connect to your router: %v", err)
	}
	err = d.Clear(port)
	if check(err) {
		return err
	}
	return nil
}

func getUpnpPublicAddress(routerAddress string) (ipAddress string, err error) {
	// connect UPnP to router
	d, err := upnp.Load(routerAddress)
	if check(err) {
		err = fmt.Errorf("upnp is unable to connect to your router: %v", err)
		return
	}
	ipAddress, err = d.ExternalIP()
	return
}

func getPublicIPAddress() (ipAddress string, err error) {
	// do a parallel query of 4 services to ensure if one is blocked we can get an address
	m := pubip.NewMaster()
	m.Parallel = 4
	m.Format = pubip.IPv4
	ipAddress, err = m.Address()
	return
}
