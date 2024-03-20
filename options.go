package roller

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Options represents optional behavior you can specify for a new Roller.
type Options struct {
	Filename          string
	Size              int64
	Duration          time.Duration
	RotateName        RotateNameFunc
	LifecycleGlob     string
	LifecycleSize     int64
	LifecycleCount    int64
	LifecycleDuration time.Duration
}

type Option func(*Options)

type RotateNameFunc func(string) string

func newOptions(opt ...Option) *Options {
	options := &Options{
		RotateName: defaultRotateName,
	}
	for _, o := range opt {
		o(options)
	}
	return options
}

func Filename(n string) Option {
	return func(o *Options) {
		o.Filename = n
	}
}

func Size(n int64) Option {
	return func(o *Options) {
		o.Size = n
	}
}

func Duration(d time.Duration) Option {
	return func(o *Options) {
		o.Duration = d
	}
}

func RotateName(fn RotateNameFunc) Option {
	return func(o *Options) {
		o.RotateName = fn
	}
}

func LifecycleGlob(glob string) Option {
	return func(o *Options) {
		o.LifecycleGlob = glob
	}
}

func LifecycleSize(s int64) Option {
	return func(o *Options) {
		o.LifecycleSize = s
	}
}

func LifecycleCount(c int64) Option {
	return func(o *Options) {
		o.LifecycleCount = c
	}
}

func LifecycleDuration(d time.Duration) Option {
	return func(o *Options) {
		o.LifecycleDuration = d
	}
}

func defaultRotateName(filename string) string {
	dir := filepath.Dir(filename)
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filepath.Base(filename), ext)

	return path.Join(dir, fmt.Sprintf("%s_%s%s", name, time.Now().Format("2006-01-02T15:04:05.999Z07:00"), ext))
}
