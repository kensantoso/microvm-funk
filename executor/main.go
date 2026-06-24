// Command executor runs INSIDE a Lambda MicroVM as the always-up application on
// :8080. It exposes a single endpoint, /exec, which runs an arbitrary command
// via `bash -lc` and returns its combined output as text/plain. This is the
// fast iteration / control channel kube-minimal uses (node-join, kubectl, etc.)
// — it's already behind the MicroVM auth token.
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

func main() {
	mux := http.NewServeMux()

	// /exec runs an arbitrary command via `bash -lc` and returns its combined
	// output as text/plain. The command comes from ?cmd= or the request body.
	// To keep a daemon alive across requests, detach it, e.g.:
	//   setsid sh -c 'k3s server ... >/tmp/k3s.log 2>&1 &'
	mux.HandleFunc("/exec", func(w http.ResponseWriter, req *http.Request) {
		cmdStr := req.URL.Query().Get("cmd")
		if cmdStr == "" {
			b, _ := io.ReadAll(req.Body)
			cmdStr = string(b)
		}
		w.Header().Set("Content-Type", "text/plain")
		if strings.TrimSpace(cmdStr) == "" {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "no command provided (use ?cmd=... or POST body)\n")
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 280*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, "bash", "-lc", cmdStr)
		out, err := c.CombinedOutput()
		w.Write(out)
		if err != nil {
			io.WriteString(w, "\n[exit error: "+err.Error()+"]\n")
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("catch-all hit: %s %s", r.Method, r.URL.Path)
		w.Write([]byte("lambda-microvm exec agent; POST /exec?cmd=...\n"))
	})

	log.Println("executor server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
