package snowflake

import (
	"github.com/bwmarrin/snowflake"
)

var node *snowflake.Node

func Init(nodeID int64) error {
	var err error
	node, err = snowflake.NewNode(nodeID)
	return err
}

func NewID() int64 {
	return node.Generate().Int64()
}
