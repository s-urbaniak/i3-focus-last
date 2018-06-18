package i3_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/s-urbaniak/i3-focus-last/i3"
)

type readWriter struct {
	r io.Reader
	w io.Writer
}

func (rw readWriter) Read(p []byte) (int, error) {
	return rw.r.Read(p)
}

func (rw readWriter) Write(p []byte) (int, error) {
	return rw.w.Write(p)
}

func TestCommand(t *testing.T) {
	var b bytes.Buffer
	c := i3.NewClient(&b)
	c.Subscribe("event1", "event2")
	fmt.Println(hex.Dump(b.Bytes()))
}

func TestVersion(t *testing.T) {
	sp, err := i3.Socketpath()
	if err != nil {
		t.Fatal(err)
	}

	conn, err := i3.NewConnection(sp)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var b bytes.Buffer
	rw := readWriter{
		r: io.TeeReader(conn, &b),
		w: io.MultiWriter(conn, &b),
	}

	c := i3.NewClient(rw)
	err = c.Write(i3.MsgVersion, nil)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(hex.Dump(b.Bytes()))
	b.Reset()

	fmt.Println(c.Read())
	fmt.Println(hex.Dump(b.Bytes()))
}

func TestTree(t *testing.T) {
	sp, err := i3.Socketpath()
	if err != nil {
		t.Fatal(err)
	}

	conn, err := i3.NewConnection(sp)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var b bytes.Buffer
	rw := readWriter{
		r: io.TeeReader(conn, &b),
		w: io.MultiWriter(conn, &b),
	}

	c := i3.NewClient(rw)
	root, err := c.Tree()
	if err != nil {
		t.Error(err)
	}

	fmt.Printf("%+v\n", root)
}
