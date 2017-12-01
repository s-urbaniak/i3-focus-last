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

func evLoop(evChan chan []byte, cm *conn.Manager, l log.Logger) (err error) {
	defer func() {
		if err != nil {
			l.Log("err", err)
			os.Exit(1)
		}
	}()

	c := cm.Take()
	if c == nil {
		return errors.New("no connections available")
	}

	i3c := i3.New(c)
	if err := i3c.Subscribe([]string{"window"}); err != nil {
		return errors.New("subscribe for window events failed")
	}

	for {
		ev, err := i3c.ReadMsg()
		if err != nil {
			return err
		}

		evChan <- ev
	}
}

func main() {
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)

	if len(os.Args) > 1 && os.Args[1] == "switch" {
		if err := remoteSwitch(); err != nil {
			logger.Log("err", err)
			os.Exit(1)
		}

		os.Exit(0)
	}

	switchChan := make(chan struct{})
	switchFn := func() { switchChan <- struct{}{} }

	go func() {
		if err := startServer(switchFn); err != nil {
			logger.Log("err", err)
			os.Exit(1)
		}
	}()

	var c net.Conn
	adr, err := i3.Socketpath()
	if err != nil {
		logger.Log("err", err)
		os.Exit(1)
	}

	cm := conn.NewManager(i3.Dial, "unix", adr, time.After, logger)
	c = cm.Take()
	if c == nil {
		logger.Log("err", "no connections available")
		os.Exit(1)
	}

	i3c := i3.New(c)
	root, err := i3c.Tree()
	if err != nil {
		logger.Log("err", "tree command failed")
		os.Exit(1)
	}

	history := []int{-1, -1}
	if fn := focused(root); fn != nil {
		history[1] = fn.ID
	}

	evChan := make(chan []byte)
	go evLoop(evChan, cm, logger)

	logger.Log("status", "i3-focus-last started")

	for {
		select {
		case ev := <-evChan:
			if len(ev) == 0 || ev[0] == '[' {
				continue // some other response, not a struct
			}

			evJson := struct {
				Change    string `json:"change"`
				Container struct {
					ID int `json:"id"`
				} `json:"container"`
			}{}

			if err := json.Unmarshal(ev, &evJson); err != nil {
				logger.Log("err", err)
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

			if err := i3c.Command(fmt.Sprintf("[con_id=%d] focus", history[0])); err != nil {
				logger.Log("err", err)
				os.Exit(1)
			}
		}
	}
}
