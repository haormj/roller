package roller

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type constError string

func (c constError) Error() string {
	return string(c)
}

// ErrWriteTooLong indicates that a single write that is longer than the max
// size allowed in a single file.
const ErrWriteTooLong = constError("write exceeds max file length")

// NewLumberjackRoller returns a new Roller.
//
// If the file exists and is less than maxSize bytes, lumberjack will open and
// append to that file. If the file exists and its size is >= maxSize bytes, the
// file is renamed by putting the current time in a timestamp in the name
// immediately before the file's extension (or the end of the filename if
// there's no extension). A new log file is then created using original
// filename.
//
// An error is returned if a file cannot be opened or created, or if maxsize is
// 0 or less.
func NewLumberjackRoller(opts ...Option) (Roller, error) {
	options := NewOptions(opts...)
	return NewLumberjackRollerFromOptions(options)
}

func NewLumberjackRollerFromOptions(options Options) (Roller, error) {
	if options.RotateStrategy == SizeRotateStrategy && options.FileMaxSize <= 0 {
		return nil, errors.New("max size cannot be 0")
	}
	if options.FileName == "" {
		return nil, errors.New("filename cannot be empty")
	}
	r := &LumberjackRoller{
		opts: options,
	}
	if err := r.openExistingOrNew(0); err != nil {
		return nil, fmt.Errorf("can't open file: %w", err)
	}
	return r, nil
}

// Roller wraps a file, intercepting its writes to control its size, rolling the
// old file over to a different name before writing to a new one.
//
// Whenever a write would cause the current log file exceed maxSize bytes, the
// current file is closed, renamed, and a new log file created with the original
// name. Thus, the filename you give Roller is always the "current" log file.
//
// Backups use the log file name given to Roller, in the form
// `name-timestamp.ext` where name is the filename without the extension,
// timestamp is the time at which the log was rotated formatted with the
// time.Time format of `2006-01-02T15-04-05.000` and the extension is the
// original extension. For example, if your Roller.Filename is
// `/var/log/foo/server.log`, a backup created at 6:30pm on Nov 11 2016 would
// use the filename `/var/log/foo/server-2016-11-04T18-30-00.000.log`
//
// # Cleaning Up Old Log Files
//
// Whenever a new logfile gets created, old log files may be deleted. The most
// recent files according to the encoded timestamp will be retained, up to a
// number equal to MaxBackups (or all of them if MaxBackups is 0). Any files
// with an encoded timestamp older than MaxAge days are deleted, regardless of
// MaxBackups. Note that the time encoded in the timestamp is the rotation
// time, which may differ from the last time that file was written to.
//
// If MaxBackups and MaxAge are both 0, no old log files will be deleted.
type LumberjackRoller struct {
	opts Options

	size int64
	file *os.File
	mu   sync.Mutex

	millCh    chan bool
	startMill sync.Once
}

var (
	// currentTime exists so it can be mocked out by tests.
	currentTime = time.Now

	// os_Stat exists so it can be mocked out by tests.
	osStat = os.Stat
)

// Write implements io.Writer.  If a write would cause the log file to be larger
// than MaxSize, the file is closed, renamed to include a timestamp of the
// current time, and a new log file is created using the original log file name.
// If the length of the write is greater than MaxSize, an error is returned.
func (r *LumberjackRoller) Write(p []byte) (n int, err error) {
	writeLen := int64(len(p))
	if r.opts.FileMaxSize > 0 && writeLen > r.opts.FileMaxSize {
		return 0, fmt.Errorf(
			"write length %d, max size %d: %w", writeLen, r.opts.FileMaxSize, ErrWriteTooLong,
		)
	}

	defer r.mu.Unlock()
	r.mu.Lock()

	switch r.opts.RotateStrategy {
	case SizeRotateStrategy:
		if r.size+writeLen > r.opts.FileMaxSize {
			if err := r.rotate(); err != nil {
				return 0, err
			}
		}
		n, err = r.file.Write(p)
		r.size += int64(n)
	case DirectRotateStrategy:
		n, err = r.file.Write(p)
		r.size += int64(n)
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	return n, err
}

// Close implements io.Closer, and closes the current logfile.
func (r *LumberjackRoller) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.close()
}

// close closes the file if it is open.
func (r *LumberjackRoller) close() error {
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

// Rotate causes Logger to close the existing log file and immediately create a
// new one.  This is a helper function for applications that want to initiate
// rotations outside of the normal rotation rules, such as in response to
// SIGHUP.  After rotating, this initiates compression and removal of old log
// files according to the configuration.
func (r *LumberjackRoller) Rotate() error {
	defer r.mu.Unlock()
	r.mu.Lock()
	return r.rotate()
}

// rotate closes the current file, moves it aside with a timestamp in the name,
// (if it exists), opens a new file with the original filename, and then runs
// post-rotation processing and removar.
func (r *LumberjackRoller) rotate() error {
	if err := r.close(); err != nil {
		return err
	}
	if err := r.openNew(); err != nil {
		return err
	}
	r.mill()
	return nil
}

// openNew opens a new log file for writing, moving any old log file out of the
// way.  This methods assumes the file has already been closed.
func (r *LumberjackRoller) openNew() error {
	err := os.MkdirAll(r.dir(), 0755)
	if err != nil {
		return fmt.Errorf("can't make directories for new logfile: %w", err)
	}

	name := r.newFilename()
	mode := os.FileMode(0600)
	info, err := osStat(name)
	if err == nil {
		// Copy the mode off the old logfile.
		mode = info.Mode()
		// move the existing file
		newname := r.backupName(name, r.opts.BackupTimeLocation)
		if err := os.Rename(name, newname); err != nil {
			return fmt.Errorf("can't rename log file: %w", err)
		}

		// this is a no-op anywhere but linux
		if err := chown(name, info); err != nil {
			return err
		}
	}

	// we use truncate here because this should only get called when we've moved
	// the file ourselves. if someone else creates the file in the meantime,
	// just wipe out the contents.
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("can't open new logfile: %w", err)
	}
	r.file = f
	r.size = 0
	return nil
}

// backupName creates a new filename from the given name, inserting a timestamp
// between the filename and the extension, using the local time if requested
// (otherwise UTC).
func (r *LumberjackRoller) backupName(name string, l *time.Location) string {
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	t := currentTime().In(l)

	timestamp := t.Format(r.opts.BackupTimeFormat)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, timestamp, ext))
}

// openExistingOrNew opens the logfile if it exists and if the current write
// would not put it over MaxSize.  If there is no such file or the write would
// put it over the MaxSize, a new file is created.
func (r *LumberjackRoller) openExistingOrNew(writeLen int64) error {
	r.mill()

	filename := r.newFilename()
	info, err := osStat(filename)
	if os.IsNotExist(err) {
		return r.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %w", err)
	}

	switch r.opts.RotateStrategy {
	case SizeRotateStrategy:
		if info.Size()+writeLen >= r.opts.FileMaxSize {
			return r.rotate()
		}
	case DirectRotateStrategy:
		if info.Size() > 0 {
			return r.rotate()
		}
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// if we fail to open the old log file for some reason, just ignore
		// it and open a new log file.
		return r.openNew()
	}
	r.file = file
	r.size = info.Size()
	return nil
}

// newFilename generates the name of the logfile from the current time.
func (r *LumberjackRoller) newFilename() string {
	if r.opts.FileName != "" {
		return r.opts.FileName
	}
	name := filepath.Base(os.Args[0]) + "-lumberjack.log"
	return filepath.Join(os.TempDir(), name)
}

// millRunOnce performs compression and removal of stale log files.
// Log files are compressed if enabled via configuration and old log
// files are removed, keeping at most r.MaxBackups files, as long as
// none of them are older than MaxAge.
func (r *LumberjackRoller) millRunOnce() error {
	if r.opts.MaxSize == 0 && r.opts.FileMaxCount == 0 && r.opts.FileMaxAge == 0 && !r.opts.Compress {
		return nil
	}

	files, err := r.oldLogFiles()
	if err != nil {
		return err
	}

	var compress, remove []logInfo

	if r.opts.MaxSize > 0 {
		var remaining []logInfo
		var total int64
		for _, f := range files {
			total += f.Size()
			if total > r.opts.MaxSize {
				remove = append(remove, f)
			} else {
				remaining = append(remaining, f)
			}
		}
		files = remaining
	}
	if r.opts.FileMaxCount > 0 && r.opts.FileMaxCount < len(files) {
		preserved := make(map[string]bool)
		var remaining []logInfo
		for _, f := range files {
			// Only count the uncompressed log file or the
			// compressed log file, not both.
			fn := strings.TrimSuffix(f.Name(), r.opts.CompressSuffix)
			preserved[fn] = true

			if len(preserved) > r.opts.FileMaxCount {
				remove = append(remove, f)
			} else {
				remaining = append(remaining, f)
			}
		}
		files = remaining
	}
	if r.opts.FileMaxAge > 0 {
		cutoff := currentTime().Add(-1 * time.Duration(r.opts.FileMaxAge))

		var remaining []logInfo
		for _, f := range files {
			if f.timestamp.Before(cutoff) {
				remove = append(remove, f)
			} else {
				remaining = append(remaining, f)
			}
		}
		files = remaining
	}

	if r.opts.Compress {
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), r.opts.CompressSuffix) {
				compress = append(compress, f)
			}
		}
	}

	for _, f := range remove {
		errRemove := os.Remove(filepath.Join(r.dir(), f.Name()))
		if err == nil && errRemove != nil {
			err = errRemove
		}
	}
	for _, f := range compress {
		fn := filepath.Join(r.dir(), f.Name())
		errCompress := r.compressLogFile(fn, fn+r.opts.CompressSuffix)
		if err == nil && errCompress != nil {
			err = errCompress
		}
	}

	return err
}

// millRun runs in a goroutine to manage post-rotation compression and removal
// of old log files.
func (r *LumberjackRoller) millRun() {
	for range r.millCh {
		// what am I going to do, log this?
		_ = r.millRunOnce()
	}
}

// mill performs post-rotation compression and removal of stale log files,
// starting the mill goroutine if necessary.
func (r *LumberjackRoller) mill() {
	r.startMill.Do(func() {
		r.millCh = make(chan bool, 1)
		go r.millRun()
	})
	select {
	case r.millCh <- true:
	default:
	}
}

// oldLogFiles returns the list of backup log files stored in the same
// directory as the current log file, sorted by ModTime
func (r *LumberjackRoller) oldLogFiles() ([]logInfo, error) {
	files, err := ioutil.ReadDir(r.dir())
	if err != nil {
		return nil, fmt.Errorf("can't read log file directory: %w", err)
	}
	logFiles := []logInfo{}

	prefix, ext := r.prefixAndExt()

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if t, err := r.timeFromName(f.Name(), prefix, ext); err == nil {
			logFiles = append(logFiles, logInfo{t, f})
			continue
		}
		if t, err := r.timeFromName(f.Name(), prefix, ext+r.opts.CompressSuffix); err == nil {
			logFiles = append(logFiles, logInfo{t, f})
			continue
		}
		// error parsing means that the suffix at the end was not generated
		// by lumberjack, and therefore it's not a backup file.
	}

	sort.Sort(byFormatTime(logFiles))

	return logFiles, nil
}

// timeFromName extracts the formatted time from the filename by stripping off
// the filename's prefix and extension. This prevents someone's filename from
// confusing time.parse.
func (r *LumberjackRoller) timeFromName(filename, prefix, ext string) (time.Time, error) {
	if !strings.HasPrefix(filename, prefix) {
		return time.Time{}, errors.New("mismatched prefix")
	}
	if !strings.HasSuffix(filename, ext) {
		return time.Time{}, errors.New("mismatched extension")
	}
	ts := filename[len(prefix) : len(filename)-len(ext)]
	return time.Parse(r.opts.BackupTimeFormat, ts)
}

// dir returns the directory for the current filename.
func (r *LumberjackRoller) dir() string {
	return filepath.Dir(r.newFilename())
}

// prefixAndExt returns the filename part and extension part from the Logger's
// filename.
func (r *LumberjackRoller) prefixAndExt() (prefix, ext string) {
	filename := filepath.Base(r.newFilename())
	ext = filepath.Ext(filename)
	prefix = filename[:len(filename)-len(ext)] + "-"
	return prefix, ext
}

// compressLogFile compresses the given log file, removing the
// uncompressed log file if successfur.
func (r *LumberjackRoller) compressLogFile(src, dst string) (err error) {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	fi, err := osStat(src)
	if err != nil {
		return fmt.Errorf("failed to stat log file: %w", err)
	}

	if err := chown(dst, fi); err != nil {
		return fmt.Errorf("failed to chown compressed log file: %w", err)
	}

	// If this file already exists, we presume it was created by
	// a previous attempt to compress the log file.
	gzf, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fi.Mode())
	if err != nil {
		return fmt.Errorf("failed to open compressed log file: %w", err)
	}
	defer gzf.Close()

	gz := gzip.NewWriter(gzf)

	defer func() {
		if err != nil {
			os.Remove(dst)
			err = fmt.Errorf("failed to compress log file: %w", err)
		}
	}()

	if _, err := io.Copy(gz, f); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := gzf.Close(); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return err
	}

	return nil
}

// logInfo is a convenience struct to return the filename and its embedded
// timestamp.
type logInfo struct {
	timestamp time.Time
	os.FileInfo
}

// byFormatTime sorts by newest time formatted in the name.
type byFormatTime []logInfo

func (b byFormatTime) Less(i, j int) bool {
	return b[i].timestamp.After(b[j].timestamp)
}

func (b byFormatTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byFormatTime) Len() int {
	return len(b)
}
