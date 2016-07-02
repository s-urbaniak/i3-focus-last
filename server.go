package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
)

type switchFunc func()

const socketTpl = "\x00i3-focus-last/%d"

func startServer(sf switchFunc) error {
	l, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: fmt.Sprintf(socketTpl, os.Getuid()),
		Net:  "unix",
	})

	if err != nil {
		return err
	}

	for {
		func() {
			conn, err := l.AcceptUnix()
			if err != nil {
				log.Println(err)
				return
			}
			defer conn.Close()

			b, err := ioutil.ReadAll(io.LimitReader(conn, 1))
			if b[0] != 's' {
				log.Println("invalid command")
				return
			}

			sf()
		}()
	}
}

func remoteSwitch() error {
	addr := net.UnixAddr{
		Name: fmt.Sprintf(socketTpl, os.Getuid()),
		Net:  "unix",
	}

	conn, err := net.DialUnix("unix", nil, &addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.Write([]byte{'s'}); err != nil {
		return err
	}

	return nil
}
