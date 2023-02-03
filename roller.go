package roller

import (
	"io"
	"time"
)

type Roller interface {
	io.Writer
	io.Closer
	Rotate() error
}

type Option func(*Options)

type RotateStrategy int32

const (
	SizeRotateStrategy   RotateStrategy = 0
	DirectRotateStrategy RotateStrategy = 1
)

var (
	DefaultCompressSuffix     = ".gz"
	DefaultBackupTimeFormat   = "2006-01-02T15-04-05.000"
	DefaultBackupTimeLocation = time.Local
)
