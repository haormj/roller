package roller

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/haormj/fileshredder"
)

// ErrWriteTooLong indicates that a single write that is longer than the max
// size allowed in a single file.
var ErrWriteTooLong = errors.New("write exceeds max file length")

type Roller struct {
	lock         sync.Mutex
	options      *Options
	f            *os.File
	size         int64
	createTime   time.Time
	millCh       chan bool
	fileShredder *fileshredder.FileShredder
}

func NewRoller(opts ...Option) (*Roller, error) {
	options := newOptions(opts...)
	if len(options.Filename) == 0 {
		return nil, errors.New("filename can not empty")
	}

	r := &Roller{
		options: options,
		millCh:  make(chan bool),
	}

	if len(options.LifecycleGlob) > 0 &&
		(options.LifecycleSize != 0 || options.LifecycleCount != 0 || options.LifecycleDuration != 0) {
		fs, err := fileshredder.NewFileShredder(
			fileshredder.GlobPath(options.LifecycleGlob),
			fileshredder.MaxSize(options.LifecycleSize),
			fileshredder.MaxAge(options.LifecycleDuration),
			fileshredder.MaxCount(options.LifecycleCount),
		)
		if err != nil {
			return nil, err
		}
		r.fileShredder = fs
		go r.millRun()
	}

	if err := r.open(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Roller) Write(p []byte) (n int, err error) {
	writeLen := int64(len(p))
	if r.options.Size > 0 && writeLen > r.options.Size {
		return 0, fmt.Errorf("write length %d, max size %d: %w", writeLen, r.options.Size, ErrWriteTooLong)
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.isRotate(writeLen) {
		if err = r.rotate(); err != nil {
			return
		}
	}

	n, err = r.f.Write(p)
	r.size += int64(n)

	return
}

func (r *Roller) Close() error {
	r.lock.Lock()
	defer r.lock.Unlock()

	close(r.millCh)

	return r.close()
}

func (r *Roller) Rotate() error {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.rotate()
}

func (r *Roller) open() error {
	var f *os.File
	var err error
	_, err = os.Stat(r.options.Filename)
	switch {
	case err == nil:
		f, err = r.openExist()
	case os.IsNotExist(err):
		f, err = r.openNew()
	default:
		return err
	}

	if err != nil {
		return err
	}

	info, err := f.Stat()
	if err != nil {
		return err
	}

	r.f = f
	r.size = info.Size()
	r.createTime = info.ModTime()

	return nil
}

func (r *Roller) openNew() (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(r.options.Filename), 0755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(r.options.Filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (r *Roller) openExist() (*os.File, error) {
	return os.OpenFile(r.options.Filename, os.O_APPEND|os.O_WRONLY, 0644)
}

func (r *Roller) close() error {
	if r.f == nil {
		return nil
	}

	defer func() {
		r.f = nil
		r.size = 0
		r.createTime = time.Time{}
	}()

	return r.f.Close()
}

func (r *Roller) rotate() error {
	if err := r.close(); err != nil {
		return err
	}

	rotateName := r.options.RotateName(r.options.Filename)
	if err := os.MkdirAll(filepath.Dir(rotateName), 0755); err != nil {
		return err
	}
	if err := os.Rename(r.options.Filename, rotateName); err != nil {
		return err
	}

	if err := r.open(); err != nil {
		return err
	}

	r.mill()

	return nil
}

func (r *Roller) isRotate(writeLen int64) bool {
	if r.options.Size > 0 && writeLen+r.size > r.options.Size {
		return true
	}

	if r.options.Duration > 0 && time.Since(r.createTime) > r.options.Duration {
		return true
	}

	return false
}

func (r *Roller) mill() {
	select {
	case r.millCh <- true:
	default:
	}
}

func (r *Roller) millRun() {
	for range r.millCh {
		if err := r.fileShredder.MillRunOnce(); err != nil {
			log.Println("MillRunOnce err", err)
		}
	}
}
