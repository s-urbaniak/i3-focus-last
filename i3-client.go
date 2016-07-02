package main

import (
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"os/exec"
)

type msgType uint32

const (
	command    msgType = 0
	workspaces msgType = 1
	subscribe  msgType = 2
	outputs    msgType = 3
	tree       msgType = 4
	marks      msgType = 5
	barConfig  msgType = 6
	version    msgType = 7
)

type i3Client struct {
	rwc io.ReadWriteCloser
}

type msg struct {
	Magic [6]byte
	Len   uint32
	Typ   msgType
}

func (c *i3Client) Connect() error {
	out, err := exec.Command("i3", "--get-socketpath").Output()
	if err != nil {
		return err
	}

	addr := net.UnixAddr{
		Name: string(out[:len(out)-1]), // omit newline
		Net:  "unix",
	}

	conn, err := net.DialUnix("unix", nil, &addr)
	if err != nil {
		return err
	}

	c.rwc = conn
	return nil
}

func (c *i3Client) Close() error {
	return c.rwc.Close()
}

func (c *i3Client) msg(t msgType, payload []byte) ([]byte, error) {
	if err := c.tx(t, payload); err != nil {
		return nil, err
	}

	return c.rx()
}

func (c *i3Client) tx(t msgType, payload []byte) error {
	req := msg{
		Magic: [6]byte{'i', '3', '-', 'i', 'p', 'c'},
		Len:   uint32(len(payload)),
		Typ:   t,
	}

	if err := binary.Write(c.rwc, binary.LittleEndian, req); err != nil {
		return err
	}

	if _, err := c.rwc.Write(payload); err != nil {
		return err
	}

	return nil
}

func (c *i3Client) rx() ([]byte, error) {
	var res msg
	if err := binary.Read(c.rwc, binary.LittleEndian, &res); err != nil {
		return nil, err
	}

	if string(res.Magic[:]) != "i3-ipc" {
		return nil, errors.New("invalid magic response")
	}

	b, err := ioutil.ReadAll(io.LimitReader(c.rwc, int64(res.Len)))
	if err != nil {
		return nil, err
	}

	return b, nil
}
