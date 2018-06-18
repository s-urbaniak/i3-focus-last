package i3

func (c Client) Command(s string) error {
	return c.Write(MsgCommand, []byte(s))
}
