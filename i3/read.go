package i3

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/pkg/errors"
)

func (c Client) Read() (MsgType, []byte, error) {
	var (
		buf = make([]byte, 6) // 6 bytes = len("i3-ipc")
		err error
	)

	read := func(e string) {
		if err != nil {
			return
		}

		_, err = io.ReadFull(c.rw, buf)

		if err != nil {
			err = fmt.Errorf(e, err)
		}
	}

	read("error reading magic string: %v")
	magic := string(buf)

	buf = buf[:4] // 4 bytes = len(uint32)
	read("error reading length: %v")
	len := binary.LittleEndian.Uint32(buf)

	read("error reading type: %v")
	typ := binary.LittleEndian.Uint32(buf)

	if err != nil {
		return 0, nil, err
	}

	if magic != "i3-ipc" {
		return 0, nil, errors.New("invalid magic string")
	}

	buf = make([]byte, int(len)) // allocate payload size
	read("error reading payload: %v")

	return MsgType(typ), buf, err
}
