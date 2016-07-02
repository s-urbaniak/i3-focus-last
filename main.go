package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type node struct {
	ID      int    `json:"id"`
	Focused bool   `json:"focused"`
	Nodes   []node `json:"nodes"`
}

func focused(root *node) *node {
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

func main() {
	if len(os.Args) > 1 && os.Args[1] == "switch" {
		if err := remoteSwitch(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	switchChan := make(chan struct{})
	switchFn := func() { switchChan <- struct{}{} }

	go func() {
		if err := startServer(switchFn); err != nil {
			log.Fatal(err)
		}
	}()

	var c i3Client
	if err := c.Connect(); err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	if err := c.tx(tree, nil); err != nil {
		log.Fatal(err)
	}

	tree, err := c.rx()
	if err != nil {
		log.Fatal(err)
	}

	var root node
	if err := json.Unmarshal(tree, &root); err != nil {
		log.Fatal(err)
	}

	history := []int{-1, -1}
	if fn := focused(&root); fn != nil {
		history[1] = fn.ID
	}

	if err := c.tx(subscribe, []byte(`["window"]`)); err != nil {
		log.Fatal(err)
	}

	evChan := make(chan []byte)
	go func() {
		for {
			ev, err := c.rx()
			if err != nil {
				log.Fatal(err)
			}
			evChan <- ev
		}
	}()

	for {
		select {
		case ev := <-evChan:
			if ev[0] == '[' {
				break // some other response, not a struct
			}

			evJson := struct {
				Change    string `json:"change"`
				Container struct {
					ID int `json:"id"`
				} `json:"container"`
			}{}

			if err := json.Unmarshal(ev, &evJson); err != nil {
				log.Println(err)
				continue
			}

			if evJson.Change != "focus" {
				continue
			}

			history[0] = history[1]
			history[1] = evJson.Container.ID
		case <-switchChan:
			if history[0] < 0 {
				break
			}

			cmd := fmt.Sprintf("[con_id=%d] focus", history[0])
			if err := c.tx(command, []byte(cmd)); err != nil {
				log.Fatal(err)
			}
		}
	}
}
