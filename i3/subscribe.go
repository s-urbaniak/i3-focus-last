package i3

import "strings"

func (c *Client) Subscribe(events ...string) error {
	e := make([]string, len(events))
	for i := range events {
		e[i] = `"` + events[i] + `"`
	}

	s := "[" + strings.Join(e, ",") + "]"
	return c.Write(MsgSubscribe, []byte(s))
}
