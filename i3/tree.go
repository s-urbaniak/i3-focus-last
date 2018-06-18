package i3

import (
	"encoding/json"

	"github.com/pkg/errors"
)

type Node struct {
	ID      int    `json:"id"`
	Focused bool   `json:"focused"`
	Nodes   []Node `json:"nodes"`
}

func (c *Client) Tree() (*Node, error) {
	err := c.Write(MsgTree, nil)
	if err != nil {
		return nil, errors.Wrap(err, "tree request failed")
	}

	_, rawTree, err := c.Read()
	if err != nil {
		return nil, errors.Wrap(err, "raw tree read failed")
	}

	var root Node
	if err := json.Unmarshal(rawTree, &root); err != nil {
		return nil, errors.Wrap(err, "tree unmarshal failed")
	}

	return &root, nil
}
