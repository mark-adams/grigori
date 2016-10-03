package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

var child *exec.Cmd

func handleSignals() {
	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	for s := range sigs {
		err := child.Process.Signal(s)
		if err != nil {
			log.Printf("failed to pass signal '%s' to child: %s", s, err)
		}
		log.Printf("passed signal '%s' to child %d", s, child.Process.Pid)
	}
}

func healthcheck(w http.ResponseWriter, r *http.Request) {
	if child == nil || child.Process == nil {
		w.WriteHeader(500)
		w.Write([]byte("child process not started yet"))
		return
	}

	if child.ProcessState != nil {
		w.WriteHeader(500)
		w.Write([]byte("child process is no longer running"))
		return
	}

	w.WriteHeader(200)
	w.Write([]byte("child process is running"))
}

var (
	healthcheckPort = flag.Int("healthport", 8080, "the port to serve the healthcheck endpoint on")
	passSignals     = flag.Bool("passSignals", true, "pass termination signals to child process")
)

func main() {
	flag.Parse()
	args := flag.Args()

	log.SetPrefix("grigori: ")
	log.SetFlags(0)

	if len(args) == 0 {
		log.Printf("oops! looks like you forgot to specify a command to run!")
		os.Exit(1)
	}

	if *healthcheckPort != 0 {
		go func() {
			http.HandleFunc("/_healthcheck", healthcheck)
			err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", *healthcheckPort), nil)
			if err != nil {
				log.Fatalf("couldn't serve healthcheck: %s", err)
			}
		}()
	}

	child = exec.Command(args[0], args[1:]...)

	stdout, err := child.StdoutPipe()
	if err != nil {
		log.Fatalf("could not read from child stdout: %s", err)
	}
	stderr, err := child.StderrPipe()
	if err != nil {
		log.Fatalf("could not read from child stderr: %s", err)
	}

	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	if err = child.Start(); err != nil {
		log.Fatalf("could not run command: %s", err)
	}
	if *passSignals {
		go handleSignals()
	}

	returnCode := 0
	err = child.Wait()
	if err, failed := err.(*exec.ExitError); failed {
		if status, ok := err.Sys().(syscall.WaitStatus); ok {
			returnCode = status.ExitStatus()
		}
	}

	log.Printf("child %d exited with rc %d", child.Process.Pid, returnCode)
}
