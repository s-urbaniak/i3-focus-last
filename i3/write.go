package i3

import "encoding/binary"

func (c Client) Write(t MsgType, p []byte) (err error) {
	write := func(data []byte) {
		if err != nil {
			return
		}

		_, err = c.rw.Write(data)
	}

	// write magic string
	write([]byte("i3-ipc"))

	b := make([]byte, 4) // 4 bytes = len(uint32)

	binary.LittleEndian.PutUint32(b, uint32(len(p)))
	write(b)

	binary.LittleEndian.PutUint32(b, uint32(t))
	write(b)

	write(p)
	return
}
