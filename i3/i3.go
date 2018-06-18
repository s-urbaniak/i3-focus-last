package i3

import (
	"io"
	"net"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

type MsgType uint32

const (
	MsgCommand    MsgType = 0
	MsgWorkspaces MsgType = 1
	MsgSubscribe  MsgType = 2
	MsgOutputs    MsgType = 3
	MsgTree       MsgType = 4
	MsgMarks      MsgType = 5
	MsgBarConfig  MsgType = 6
	MsgVersion    MsgType = 7
)

func Socketpath() (string, error) {
	out, err := exec.Command("i3", "--get-socketpath").Output()
	if err != nil {
		return "", errors.Wrap(err, "getting socket path from i3 failed")
	}

	return strings.Trim(string(out), "\n"), nil
}

func NewConnection(socketpath string) (*net.UnixConn, error) {
	addr := net.UnixAddr{
		Name: socketpath,
		Net:  "unix",
	}

	conn, err := net.DialUnix("unix", nil, &addr)
	if err != nil {
		return nil, errors.Wrap(err, "dial failed")
	}

	return conn, nil
}

type Client struct {
	rw io.ReadWriter
}

func NewClient(rw io.ReadWriter) *Client {
	return &Client{
		rw: rw,
	}
}
