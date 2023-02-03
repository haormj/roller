package roller

import "time"

// Options represents optional behavior you can specify for a new Roller.
type Options struct {
	FileName           string         `yaml:"filename"`
	FileMaxAge         Duration       `yaml:"file_max_age"`
	FileMaxCount       int            `yaml:"file_max_count"`
	MaxSize            int64          `yaml:"max_size"`
	RotateStrategy     RotateStrategy `yaml:"rotate_strategy"`
	FileMaxSize        int64          `yaml:"file_max_size"`
	Compress           bool           `yaml:"compress"`
	CompressSuffix     string         `yaml:"compress_suffix"`
	BackupTimeFormat   string         `yaml:"backup_time_format"`
	BackupTimeLocation *time.Location `yaml:"backup_time_location"`
}

// NewOptions create new roller options
func NewOptions(opt ...Option) Options {
	opts := Options{
		BackupTimeFormat:   DefaultBackupTimeFormat,
		BackupTimeLocation: DefaultBackupTimeLocation,
		CompressSuffix:     DefaultCompressSuffix,
	}
	for _, o := range opt {
		o(&opts)
	}
	return opts
}

func FileMaxAge(d time.Duration) Option {
	return func(o *Options) {
		o.FileMaxAge = Duration(d)
	}
}

func FileMaxCount(c int) Option {
	return func(o *Options) {
		o.FileMaxCount = c
	}
}

func Compress(b bool) Option {
	return func(o *Options) {
		o.Compress = b
	}
}

func BackupTimeFormat(f string) Option {
	return func(o *Options) {
		o.BackupTimeFormat = f
	}
}

func CompressSuffix(s string) Option {
	return func(o *Options) {
		o.CompressSuffix = s
	}
}

func FileMaxSize(s int64) Option {
	return func(o *Options) {
		o.FileMaxSize = s
	}
}

func FileName(n string) Option {
	return func(o *Options) {
		o.FileName = n
	}
}

func MaxSize(s int64) Option {
	return func(o *Options) {
		o.MaxSize = s
	}
}

func WithRotateStrategy(rs RotateStrategy) Option {
	return func(o *Options) {
		o.RotateStrategy = rs
	}
}

func BackupTimeLocation(l *time.Location) Option {
	return func(o *Options) {
		o.BackupTimeLocation = l
	}
}
