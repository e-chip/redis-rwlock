package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
)

type Pool interface {
	Get(ctx context.Context) (Conn, error)
}

type Conn interface {
	Eval(*Script, ...interface{}) (interface{}, error)
	Close() error
}

type Script struct {
	Src  string
	Hash string
}

func NewScript(src string) *Script {
	h := sha1.New()
	_, _ = io.WriteString(h, src)
	return &Script{
		Src:  src,
		Hash: hex.EncodeToString(h.Sum(nil)),
	}
}
