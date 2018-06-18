package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/util/conn"
	"github.com/pkg/errors"
	"github.com/s-urbaniak/i3-focus-last/i3"
)

func focused(root *i3.Node) *i3.Node {
	if root.Focused {
		return root
	}

	for _, node := range root.Nodes {
		if f := focused(&node); f != nil {
			return f
		}
	}

	return nil
}

func within(d time.Duration, f func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if time.Now().After(deadline) {
			return false
		}
		if f() {
			return true
		}
		time.Sleep(d / 10)
	}
}

type connTaker func(error) (net.Conn, error)

func newConnTaker(cm *conn.Manager, timeout time.Duration) connTaker {
	return func(err error) (net.Conn, error) {
		var conn net.Conn

		if err != nil {
			cm.Put(err)
		}

		if !within(timeout, func() bool {
			conn = cm.Take()
			return conn != nil
		}) {
			return nil, errors.New("taking connection timed out")
		}

		return conn, nil
	}
}

func switchWindow(id int, take connTaker) error {
	conn, err := take(nil)
	if err != nil {
		return fmt.Errorf("error taking connection: %v", err)
	}

	client := i3.NewClient(conn)
	if err := client.Command(fmt.Sprintf("[con_id=%d] focus", id)); err != nil {
		return fmt.Errorf("focus failed", err)
	}

	return nil
}

func evLoop(evChan chan []byte, take connTaker, l log.Logger) {
	subscribe := func(err error) *i3.Client {
		conn, err := take(err)
		if err != nil {
			l.Log("err", fmt.Errorf("error taking connection: %v", err))
			os.Exit(1)
		}

		client := i3.NewClient(conn)
		if err := client.Subscribe("window"); err != nil {
			l.Log("err", fmt.Errorf("subscribe failed: %v", err))
			os.Exit(1)
		}

		return client
	}

	client := subscribe(nil)

	for {
		_, ev, err := client.Read()

		if err != nil {
			client = subscribe(fmt.Errorf("error reading event: %v", err))
		} else {
			evChan <- ev
		}
	}
}

func newConnectionManager(logger log.Logger) (*conn.Manager, error) {
	dialer := func(_, _ string) (net.Conn, error) {
		socketpath, err := i3.Socketpath()
		if err != nil {
			return nil, fmt.Errorf("error creating socketpath: %v", err)
		}

		return i3.NewConnection(socketpath)
	}

	return conn.NewManager(dialer, "", "", time.After, log.With(logger, "component", "manager")), nil
}

func main() {
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)

	if len(os.Args) > 1 && os.Args[1] == "switch" {
		if err := remoteSwitch(); err != nil {
			logger.Log("err", fmt.Errorf("error switching: %v", err))
			os.Exit(1)
		}

		os.Exit(0)
	}

	switchChan := make(chan struct{})
	switchFn := func() { switchChan <- struct{}{} }

	go func() {
		if err := startServer(switchFn); err != nil {
			logger.Log("err", fmt.Errorf("error starting server: %v", err))
			os.Exit(1)
		}
	}()

	cm, err := newConnectionManager(logger)
	if err != nil {
		logger.Log("err", fmt.Errorf("error creating connection manager: %v", err))
		os.Exit(1)
	}

	taker := newConnTaker(cm, 3*time.Second)

	conn, err := taker(nil)
	if err != nil {
		logger.Log("err", fmt.Errorf("error taking connection: %v", err))
		os.Exit(1)
	}

	i3c := i3.NewClient(conn)
	root, err := i3c.Tree()
	if err != nil {
		logger.Log("err", fmt.Errorf("tree command failed: %v", err))
		os.Exit(1)
	}

	history := []int{-1, -1}
	if fn := focused(root); fn != nil {
		history[1] = fn.ID
	}

	evChan := make(chan []byte)
	go evLoop(evChan, taker, logger)

	logger.Log("status", "i3-focus-last started")

	for {
		select {
		case ev := <-evChan:
			logger.Log("event", string(ev))

			if len(ev) == 0 || ev[0] == '[' {
				continue // some other response, not a JSON object
			}

			evJson := struct {
				Change    string `json:"change"`
				Container struct {
					ID int `json:"id"`
				} `json:"container"`
			}{}

			if err := json.Unmarshal(ev, &evJson); err != nil {
				logger.Log("err", fmt.Errorf("error unmarshaling event: %v", err))
				continue
			}

			if evJson.Change != "focus" {
				continue
			}

			history[0], history[1] = history[1], evJson.Container.ID

		case <-switchChan:
			if history[0] < 0 {
				continue
			}

			if err := switchWindow(history[0], taker); err != nil {
				logger.Log("err", fmt.Errorf("focus command failed: %v", err))
				os.Exit(1)
			}
		}
	}
}
