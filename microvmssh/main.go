// Command microvmssh is an ssh ProxyCommand for Lambda MicroVMs. It connects to the
// native shell ingress (CreateMicrovmShellAuthToken + a WebSocket to the bare
// endpoint), drives the interactive shell to `stty raw -echo; exec sshd -i`,
// waits for sshd's banner, then splices ssh's stdin/stdout to the shell. sshd
// runs on demand with NO persistent daemon -- only AWS's native shell ingress.
//
//	ssh config:  ProxyCommand microvmssh %h
//
// Wire protocol (observed): the server sends a TEXT control frame first
// ({"type":"session_init",...}) and then BINARY frames carrying raw PTY bytes;
// our input is sent as BINARY frames. We skip non-binary control frames.
//
// The MicroVM must be launched with the managed SHELL_INGRESS ingress connector.
package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	mv "github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
	"github.com/coder/websocket"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("microvmssh: ")

	region := flag.String("region", "us-east-1", "AWS region")
	flag.Parse()
	if flag.NArg() < 1 {
		log.Fatal("usage: microvmssh <microvm-id>[.microvm]")
	}
	id := microvmID(flag.Arg(0))

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		log.Fatalf("load AWS config: %v", err)
	}
	c := mv.NewFromConfig(cfg)

	gm, err := c.GetMicrovm(ctx, &mv.GetMicrovmInput{MicrovmIdentifier: aws.String(id)})
	if err != nil {
		log.Fatalf("GetMicrovm %s: %v", id, err)
	}
	endpoint := aws.ToString(gm.Endpoint)

	tok, err := c.CreateMicrovmShellAuthToken(ctx, &mv.CreateMicrovmShellAuthTokenInput{
		MicrovmIdentifier:   aws.String(id),
		ExpirationInMinutes: aws.Int32(30),
	})
	if err != nil {
		log.Fatalf("CreateMicrovmShellAuthToken: %v", err)
	}

	hdr := http.Header{}
	for k, v := range tok.AuthToken {
		hdr.Set(k, v)
	}
	wsc, resp, err := websocket.Dial(ctx, "wss://"+endpoint, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		if resp != nil {
			log.Fatalf("ws dial: %v (http %d)", err, resp.StatusCode)
		}
		log.Fatalf("ws dial: %v", err)
	}
	wsc.SetReadLimit(-1)
	defer wsc.Close(websocket.StatusNormalClosure, "")

	s := &shell{ws: wsc}
	s.runProxyCommand()
}

// shell wraps the native-shell WebSocket. Binary frames are the PTY byte stream;
// text frames are control messages we ignore for the data path.
type shell struct {
	ws  *websocket.Conn
	buf []byte // leftover binary bytes from a previous readBinary
}

func (s *shell) write(b []byte) error {
	return s.ws.Write(context.Background(), websocket.MessageBinary, b)
}

// readBinary returns the next chunk of PTY bytes, skipping text control frames.
func (s *shell) readBinary() ([]byte, error) {
	if len(s.buf) > 0 {
		b := s.buf
		s.buf = nil
		return b, nil
	}
	for {
		typ, data, err := s.ws.Read(context.Background())
		if err != nil {
			return nil, err
		}
		if typ == websocket.MessageBinary {
			return data, nil
		}
		// text control frame (session_init, etc.) -> ignore
	}
}

// runProxyCommand bootstraps sshd -i over the interactive shell, then becomes a
// transparent pipe between ssh (stdin/stdout) and the shell WebSocket.
func (s *shell) runProxyCommand() {
	// Raw, echo-off line discipline = 8-bit-clean PTY, then replace the shell
	// with sshd in inetd mode (one connection, no daemon). AuthorizedKeysCommand
	// echoes the presented "<type> <blob>" back as an authorized_keys line, so
	// every key is authorized: IAM (the shell-token call) is the real gate and
	// the image needs no authorized_keys at all.
	const sshd = "/usr/sbin/sshd -i -e -o 'AuthorizedKeysCommand=/usr/bin/echo %t %k' -o AuthorizedKeysCommandUser=root"
	const bootstrap = "stty raw -echo; exec " + sshd + " 2>/dev/null\n"
	if err := s.write([]byte(bootstrap)); err != nil {
		log.Fatalf("write bootstrap: %v", err)
	}

	// Discard the shell prompt/echo until sshd's identification banner, then
	// re-emit the banner and everything after it.
	if err := s.discardUntil([]byte("SSH-2.0-")); err != nil {
		log.Fatalf("waiting for sshd banner: %v", err)
	}

	done := make(chan struct{}, 2)
	// ssh stdin -> shell (binary frames)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if werr := s.write(buf[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	// shell (binary frames) -> ssh stdout
	go func() {
		for {
			b, err := s.readBinary()
			if len(b) > 0 {
				os.Stdout.Write(b)
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	<-done
}

// discardUntil reads binary PTY bytes until marker appears, buffering the marker
// and any trailing bytes so the next readBinary re-emits them.
func (s *shell) discardUntil(marker []byte) error {
	var acc []byte
	for {
		b, err := s.readBinary()
		if len(b) > 0 {
			acc = append(acc, b...)
			if i := bytes.Index(acc, marker); i >= 0 {
				s.buf = append(s.buf, acc[i:]...)
				return nil
			}
			if len(acc) > len(marker) { // keep a tail in case marker straddles
				acc = acc[len(acc)-len(marker):]
			}
		}
		if err != nil {
			return err
		}
	}
}

func microvmID(host string) string {
	if i := strings.IndexByte(host, '.'); i >= 0 {
		return host[:i]
	}
	return host
}
