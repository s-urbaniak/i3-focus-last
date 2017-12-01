package i3

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
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

type msg struct {
	Magic [6]byte
	Len   uint32
	Typ   msgType
}

type client struct {
	io.ReadWriter
}

func Dial(network, address string) (net.Conn, error) {
	if network != "unix" {
		return nil, errors.New("unsupported network type")
	}

	addr := net.UnixAddr{
		Name: address,
		Net:  "unix",
	}

	conn, err := net.DialUnix(network, nil, &addr)
	if err != nil {
		return nil, errors.Wrap(err, "dial failed")
	}

	return conn, nil
}

func Socketpath() (string, error) {
	out, err := exec.Command("i3", "--get-socketpath").Output()
	if err != nil {
		return "", errors.Wrap(err, "i3 invocation failed")
	}

	return string(out[:len(out)-1]), nil // omit newline
}

func New(rw io.ReadWriter) *client {
	return &client{rw}
}

func (c *client) Subscribe(events []string) error {
	for i := range events {
		events[i] = `"` + events[i] + `"`
	}
	s := "[" + strings.Join(events, ",") + "]"

	return c.writeMsg(subscribe, []byte(s))
}

func (c *client) Command(s string) error {
	return c.writeMsg(command, []byte(s))
}

type Node struct {
	ID      int    `json:"id"`
	Focused bool   `json:"focused"`
	Nodes   []Node `json:"nodes"`
}

func (c *client) Tree() (root *Node, _ error) {
	err := c.writeMsg(tree, nil)
	if err != nil {
		return nil, errors.Wrap(err, "tree request failed")
	}

	rawTree, err := c.ReadMsg()
	if err != nil {
		return nil, errors.Wrap(err, "raw tree read failed")
	}

	root = &Node{}
	if err := json.Unmarshal(rawTree, &root); err != nil {
		return nil, errors.Wrap(err, "tree unmarshal failed")
	}

	return root, nil
}

func (c *client) ReadMsg() ([]byte, error) {
	var res msg
	if err := binary.Read(c, binary.LittleEndian, &res); err != nil {
		return nil, err
	}

	if string(res.Magic[:]) != "i3-ipc" {
		return nil, errors.New("invalid magic response")
	}

	b, err := ioutil.ReadAll(io.LimitReader(c, int64(res.Len)))
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (c *client) writeMsg(t msgType, payload []byte) error {
	req := msg{
		Magic: [6]byte{'i', '3', '-', 'i', 'p', 'c'},
		Len:   uint32(len(payload)),
		Typ:   t,
	}

	if err := binary.Write(c, binary.LittleEndian, req); err != nil {
		return err
	}

	if _, err := c.Write(payload); err != nil {
		return err
	}

	return nil
}
