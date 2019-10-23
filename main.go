package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
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
		return fmt.Errorf("focus failed: %v", err)
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

var (
	unique         = flag.Bool("unique", false, "unique windows history")
	permanence     = flag.Duration("permanence", 800*time.Millisecond, "eg. 1s or 500ms")
	historySize    = flag.Int("history-size", 80, "history size of focused windows")
	ignoreFloating = flag.Bool("ignore-floating", true, "ignore floating")
)

func main() {
	flag.Usage = flag.PrintDefaults
	flag.Parse()

	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)

	args := flag.Args()
	if len(args) > 0 {
		var cmd byte
		switch args[0] {
		case "switch":
			cmd = 's'
		case "next":
			cmd = 'n'
		case "prev":
			cmd = 'p'
		default:
			logger.Log("err", fmt.Errorf("unsupported command"))
			os.Exit(0)
		}
		if err := remoteSwitch(cmd); err != nil {
			logger.Log("err", fmt.Errorf("error switching: %v", err))
			os.Exit(1)
		}
		os.Exit(0)
	}

	lastChan := make(chan struct{})
	nextChan := make(chan struct{})
	prevChan := make(chan struct{})
	switchFn := func() { lastChan <- struct{}{} }
	nextFn := func() { nextChan <- struct{}{} }
	prevFn := func() { prevChan <- struct{}{} }

	go func() {
		if err := startServer(switchFn, nextFn, prevFn); err != nil {
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

	history := newHistory(*historySize)
	if fn := focused(root); fn != nil {
		history.push(fn.ID)
	}

	evChan := make(chan []byte)
	go evLoop(evChan, taker, logger)

	logger.Log("status", "i3-focus-last started")

	focusedEv := withPermanence(evChan, *permanence)
	// focusedEv := evChan
	for {
		select {
		case ev := <-focusedEv:
			// logger.Log("event", string(ev))

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
			fmt.Printf("change: %q\n", evJson.Change)

			if evJson.Change == "close" {
				history.remove(evJson.Container.ID)
				continue
			}

			if *ignoreFloating {

			}

			if evJson.Change != "focus" {
				continue
			}
			history.push(evJson.Container.ID)
			if *unique {
				history.unique()
			}

		case <-lastChan:
			if !history.canVisit() {
				continue
			}

			id := history.visitLast()
			if err := switchWindow(id, taker); err != nil {
				logger.Log("err", fmt.Errorf("focus command failed: %v", err))
				os.Exit(1)
			}

		case <-prevChan:
			if !history.canVisit() {
				continue
			}
			id, ok := history.visitPrev()
			if !ok {
				logger.Log("warn", fmt.Errorf("could not get previous window id"))
			}
			if err := switchWindow(id, taker); err != nil {
				logger.Log("err", fmt.Errorf("focus command failed: %v", err))
			}

		case <-nextChan:
			if !history.canVisit() {
				continue
			}
			id, ok := history.visitNext()
			if !ok {
				logger.Log("warn", fmt.Errorf("could not get previous window id"))
			}
			if err := switchWindow(id, taker); err != nil {
				logger.Log("err", fmt.Errorf("focus command failed: %v", err))
			}
		}
	}
}

func withPermanence(evCh <-chan []byte, per time.Duration) <-chan []byte {
	out := make(chan []byte)

	type change struct {
		Change    string `json:"change"`
		Container struct {
			Floating string `json:"floating"`
		} `json:"container"`
	}

	go func() {
		defer close(out)
		var hold []byte
		timer := time.NewTimer(per)
		for {
			select {
			case hold = <-evCh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}

				if len(hold) > 0 && hold[0] == '[' {
					continue
				}

				var ev change
				err := json.Unmarshal(hold, &ev)
				if err != nil {
					fmt.Println("unknown event")
				}

				if ev.Change == "close" {
					out <- hold
					continue
				}

				if *ignoreFloating && strings.Contains(ev.Container.Floating, "on") {
					continue
				}

				timer.Reset(per)

			case <-timer.C:
				// fmt.Println("\n\nwindow focused...")
				out <- hold
			}
		}
	}()

	return out
}

type history struct {
	stack []int
	cur   int
	last  int
	size  int
}

func newHistory(size int) history {
	return history{
		size: size,
	}
}

func (h *history) markLast() {
	h.last = h.cur
}

func (h *history) push(id int) {
	if len(h.stack) > 0 {
		if h.stack[h.cur] == id {
			return
		}
	} else {
		h.stack = append(h.stack, id)
		h.cur = 0
		return
	}

	h.markLast()

	h.cur++
	if h.cur == len(h.stack) {
		h.stack = append(h.stack, id)
	} else {
		// mid push
		h.stack[h.cur] = id
		h.stack = h.stack[:h.cur+1]
	}

	if len(h.stack) >= h.size {
		h.stack = h.stack[1:]
		h.cur--
	}
}

func (h *history) visitPrev() (int, bool) {
	if len(h.stack) == 0 {
		return 0, false
	}
	h.markLast()

	if h.cur == 0 {
		h.cur = len(h.stack) - 1
	} else {
		h.cur--
	}

	return h.stack[h.cur], true
}

func (h *history) visitNext() (int, bool) {
	if len(h.stack) == 0 {
		return 0, false
	}
	h.markLast()

	if h.cur == len(h.stack)-1 {
		h.cur = 0
	} else {
		h.cur++
	}

	return h.stack[h.cur], true
}

func (h *history) visitLast() int {
	ret := h.stack[h.last]
	h.last, h.cur = h.cur, h.last
	return ret
}

func (h *history) canVisit() bool {
	return len(h.stack) > 1
}

func (h *history) remove(id int) {
	var newStack []int
	var newCur, newLast int
	var lastSkip int
	for i, v := range h.stack {
		if id == v {
			if i < h.last {
				lastSkip++
			}
			continue
		}
		if h.cur < i {
			newCur++
		}
		newStack = append(newStack, v)
	}
	newLast = h.last - lastSkip

	h.stack = newStack
	h.cur = newCur
	h.last = newLast
}

func (h *history) unique() {
	if !h.canVisit() {
		return
	}

	set := make(map[int]struct{})
	var newCur int
	var newStack []int
	for i := len(h.stack) - 1; i >= 0; i-- {
		if _, ok := set[h.stack[i]]; ok {
			continue
		}
		set[h.stack[i]] = struct{}{}
		newStack = append(newStack, h.stack[i])
		if h.stack[i] == h.stack[h.cur] {
			newCur = len(newStack) - 1
		}
	}

	// reverse
	var swapped bool
	for i, j := 0, len(newStack)-1; i < j; i, j = i+1, j-1 {
		if newCur == i && !swapped {
			newCur = j
			swapped = true
		}
		newStack[i], newStack[j] = newStack[j], newStack[i]
	}

	h.stack = newStack
	h.cur = newCur
}
