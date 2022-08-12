// goup watches application dependencies for changes and restarts
// it whenever files change.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

var logger = log.New(os.Stderr, "goup -> ", 0)
var watchOps = []fsnotify.Op{fsnotify.Write, fsnotify.Create, fsnotify.Remove}
var watchExt = []string{".go"}

var termSignal = os.Getenv("GOUP_TERM_SIGNAL")

type project struct {
	Name   string
	Deps   []string
	Target string
	Dir    string

	cmd        *exec.Cmd
	restarting bool
	watched    []string
	stdin      []byte
}

func main() {
	panic(os.Getenv("GOUP_TERM_SIGNAL"))
	prj, err := read()
	if err != nil {
		logger.Fatalf("failed import: %v", err)
	}

	// Ensure term-signal flag is valid
	getTermSignal()
	logger.Printf("using termination signal: %s", termSignal)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		prj.terminate()
		os.Exit(0)
	}()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Fatalf("failed to create fsnotify watcher: %v", err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if prj.restarting || !validate(event).valid() {
					continue
				}
				prj.restarting = true
				logger.Println(event)
				go prj.restart()
			case err := <-watcher.Errors:
				logger.Println("watch error:", err)
			}
		}
	}()

	for _, pkgDir := range prj.watched {
		if err := watcher.Add(pkgDir); err != nil {
			logger.Fatalf("failed to register: %s - %v", pkgDir, err)
		}
	}

	if err := watcher.Add(prj.Dir); err != nil {
		logger.Fatalf("failed to register: %s - %v", prj.Dir, err)
	}

	prj.restarting = true
	go prj.restart()
	<-done
}

// Get the signal used to terminal signals.
func getTermSignal() syscall.Signal {
	if termSignal == "INT" {
		return syscall.SIGINT
	} else if termSignal == "TERM" || termSignal == "" {
		return syscall.SIGTERM
	} else {
		panic(fmt.Sprintf("invalid GOUP_TERM_SIGNAL value: %s", termSignal))
	}
}

func (p *project) terminate() {
	// send termination signal, to running application
	if p.cmd != nil && p.cmd.Process != nil {
		logger.Println("terminating process:", p.cmd.Process.Pid)
		if err := p.cmd.Process.Signal(getTermSignal()); err != nil {
			logger.Println("failed to terminate:", p.cmd.Process.Pid, "reason:", err)
		}
	}
}

func (p *project) restart() {
	if p.Name == "main" {
		logger.Println("restarting application")
	} else {
		logger.Println("recompiling package")
	}
	// use go install, it rebuilds only changed packages
	cmd := exec.Command("go", "install")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		logger.Println("failed to run go install -", err)
		p.restarting = false
		return // could not recompile due to error, wait for another change
	}

	if p.Name != "main" {
		p.restarting = false
		return // nothing to do for a library
	}

	p.terminate()

	var stdin io.Reader = os.Stdin
	if len(p.stdin) > 0 {
		buf := make([]byte, len(p.stdin))
		copy(buf, p.stdin)
		stdin = bytes.NewReader(buf)
	}

	p.cmd = exec.Command(p.Target, os.Args[1:]...)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stdin = stdin
	p.cmd.Stderr = os.Stderr
	if err := p.cmd.Start(); err != nil {
		logger.Println("failed to run command:", err)
		p.restarting = false
		return // could not start command
	}
	p.restarting = false
	// wait until terminated
	logger.Println("started on PID:", p.cmd.Process.Pid)
	p.cmd.Wait()
	// @TODO: maybe restart with few retries on error exit codes
}

func read() (*project, error) {
	data, err := exec.Command("go", "list", "-json", "-e").Output()
	if err != nil {
		return nil, err
	}

	prj := &project{}
	if err = json.Unmarshal(data, prj); err != nil {
		return nil, err
	}

	ps := string(os.PathSeparator)
	for _, dep := range prj.Deps {
		if strings.Index(dep, ps+"vendor"+ps) != -1 {
			continue // skip vendor
		}

		pkg, err := build.Import(dep, prj.Dir, 0)
		if err != nil {
			continue
		}

		if pkg.Goroot {
			continue
		}

		prj.watched = append(prj.watched, pkg.Dir)
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}

	if (stat.Mode() & os.ModeCharDevice) == 0 {
		prj.stdin, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
	}

	return prj, nil
}

type validate fsnotify.Event

func (v validate) valid() bool {
	return v.ops() && v.ext()
}

func (v validate) ops() bool {
	for _, op := range watchOps {
		if v.Op&op == op {
			return true
		}
	}
	return false
}

func (v validate) ext() bool {
	ext := filepath.Ext(v.Name)
	for _, e := range watchExt {
		if ext == e {
			return true
		}
	}
	return false
}
