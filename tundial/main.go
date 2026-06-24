// Command tundial opens a full-duplex tunnel to an in-VM TCP port through the
// MicroVM HTTP ingress endpoint, by speaking WebSocket to the in-VM agent's
// /tunnel handler. It mints a fresh MicroVM auth token each time.
//
//	-listen ADDR     : accept local TCP connections and bridge each one to a
//	                   fresh tunnel to the in-VM target port.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	mv "github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms/types"
	"github.com/coder/websocket"
)

func main() {
	region := flag.String("region", "us-east-1", "AWS region")
	id := flag.String("microvm-id", "", "MicroVM id")
	agentPort := flag.Int("agent-port", 8081, "in-VM port the tunnel agent listens on")
	targetPort := flag.Int("target-port", 9999, "in-VM TCP port to tunnel to (e.g. 6443 apiserver)")
	listen := flag.String("listen", "", "local TCP address to accept and bridge (e.g. 127.0.0.1:7000)")
	flag.Parse()
	if *id == "" {
		log.Fatal("-microvm-id is required")
	}
	if *listen == "" {
		log.Fatal("-listen ADDR is required")
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		log.Fatal(err)
	}
	c := mv.NewFromConfig(cfg)
	gm, err := c.GetMicrovm(ctx, &mv.GetMicrovmInput{MicrovmIdentifier: aws.String(*id)})
	if err != nil {
		log.Fatalf("GetMicrovm: %v", err)
	}
	endpoint := aws.ToString(gm.Endpoint)
	dial := func() (net.Conn, error) {
		return dialTunnel(ctx, c, *id, endpoint, *agentPort, *targetPort)
	}

	serveListener(*listen, dial)
}

// dialTunnel mints a token and opens a WebSocket to the agent /tunnel handler,
// returning it as a net.Conn carrying raw bytes to the in-VM target port.
func dialTunnel(ctx context.Context, c *mv.Client, id, endpoint string, agentPort, targetPort int) (net.Conn, error) {
	tok, err := c.CreateMicrovmAuthToken(ctx, &mv.CreateMicrovmAuthTokenInput{
		MicrovmIdentifier:   aws.String(id),
		ExpirationInMinutes: aws.Int32(30),
		AllowedPorts:        []types.PortSpecification{&types.PortSpecificationMemberPort{Value: int32(agentPort)}},
	})
	if err != nil {
		return nil, fmt.Errorf("CreateMicrovmAuthToken: %w", err)
	}
	authToken := tok.AuthToken["X-aws-proxy-auth"]

	u := fmt.Sprintf("wss://%s/tunnel?port=%d", endpoint, targetPort)
	hdr := http.Header{}
	hdr.Set("X-aws-proxy-auth", authToken)
	hdr.Set("X-aws-proxy-port", fmt.Sprintf("%d", agentPort))

	wsConn, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("ws dial: %w (http %d)", err, resp.StatusCode)
		}
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	wsConn.SetReadLimit(-1)
	return websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary), nil
}

func serveListener(addr string, dial func() (net.Conn, error)) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	log.Printf("tundial: listening on %s, bridging to in-VM target", addr)
	var connID int64
	for {
		local, err := ln.Accept()
		if err != nil {
			log.Fatalf("accept: %v", err)
		}
		id := atomic.AddInt64(&connID, 1)
		log.Printf("tundial: conn#%d accepted from %s", id, local.RemoteAddr())
		go func(l net.Conn, id int64) {
			start := time.Now()
			remote, err := dial()
			if err != nil {
				log.Printf("tundial: conn#%d dial tunnel: %v", id, err)
				l.Close()
				return
			}
			log.Printf("tundial: conn#%d tunnel established in %s", id, time.Since(start))
			done := make(chan struct{}, 2)
			go func() { io.Copy(l, remote); done <- struct{}{} }()
			go func() { io.Copy(remote, l); done <- struct{}{} }()
			<-done
			l.Close()
			remote.Close()
			log.Printf("tundial: conn#%d closed after %s", id, time.Since(start))
		}(local, id)
	}
}
