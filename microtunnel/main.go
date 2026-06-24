// Command microtunnel runs INSIDE a Lambda MicroVM and provides a full-duplex
// byte tunnel from the MicroVM's HTTP ingress endpoint to an arbitrary in-VM TCP
// port (e.g. kubelet :10250). The tunnel rides a WebSocket, which is the
// streaming primitive most likely to survive the MicroVM's L7 HTTP proxy.
//
//	GET /tunnel?port=N   (WebSocket upgrade)  <->  127.0.0.1:N
//
// Run it in the VM on a spare port and mint a MicroVM auth token for that port:
//
//	setsid /tmp/microtunnel -addr :8081 >/tmp/microtunnel.log 2>&1 &
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

func main() {
	addr := flag.String("addr", ":8081", "listen address for the tunnel HTTP server")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", tunnelHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "microtunnel ok\n")
	})

	log.Printf("microtunnel: tunnel server listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// tunnelHandler upgrades to a WebSocket and splices its binary stream to a TCP
// connection to 127.0.0.1:<port> inside the VM.
func tunnelHandler(w http.ResponseWriter, r *http.Request) {
	port := r.URL.Query().Get("port")
	if port == "" {
		http.Error(w, "missing ?port=", http.StatusBadRequest)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The MicroVM proxy presents its own Host; don't reject on origin.
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("tunnel: accept: %v", err)
		return
	}
	// No deadline: tunnels are long-lived (watch streams, exec sessions).
	ctx := context.Background()
	c.SetReadLimit(-1)
	netConn := websocket.NetConn(ctx, c, websocket.MessageBinary)

	addr := net.JoinHostPort("127.0.0.1", port)
	target, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		log.Printf("tunnel: dial %s: %v", addr, err)
		c.Close(websocket.StatusInternalError, "dial failed")
		return
	}
	log.Printf("tunnel: bridging ws <-> %s", addr)
	bridge(netConn, target)
	c.Close(websocket.StatusNormalClosure, "done")
}

// bridge copies bytes in both directions until either side closes.
func bridge(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { io.Copy(a, b); done <- struct{}{} }()
	go func() { io.Copy(b, a); done <- struct{}{} }()
	<-done
	a.Close()
	b.Close()
}
