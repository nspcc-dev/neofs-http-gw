package logger

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	// Option represents logger option setter.
	Option func(o *options)

	options struct {
		Options []zap.Option

		SamplingInitial    int
		SamplingThereafter int

		Format     string
		Level      string
		TraceLevel string

		NoCaller     bool
		NoDisclaimer bool

		AppName    string
		AppVersion string
	}
)

const (
	formatJSON    = "json"
	formatConsole = "console"

	defaultSamplingInitial    = 100
	defaultSamplingThereafter = 100

	lvlInfo  = "info"
	lvlWarn  = "warn"
	lvlDebug = "debug"
	lvlError = "error"
	lvlFatal = "fatal"
	lvlPanic = "panic"
)

func safeLevel(lvl string) zap.AtomicLevel {
	switch strings.ToLower(lvl) {
	case lvlDebug:
		return zap.NewAtomicLevelAt(zap.DebugLevel)
	case lvlWarn:
		return zap.NewAtomicLevelAt(zap.WarnLevel)
	case lvlError:
		return zap.NewAtomicLevelAt(zap.ErrorLevel)
	case lvlFatal:
		return zap.NewAtomicLevelAt(zap.FatalLevel)
	case lvlPanic:
		return zap.NewAtomicLevelAt(zap.PanicLevel)
	default:
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	}
}

func defaults() *options {
	return &options{
		SamplingInitial:    defaultSamplingInitial,
		SamplingThereafter: defaultSamplingThereafter,

		Format:     formatConsole,
		Level:      lvlDebug,
		TraceLevel: lvlInfo,

		NoCaller:     false,
		NoDisclaimer: false,

		AppName:    "",
		AppVersion: "",
	}
}

// New returns new zap.Logger using all options specified and stdout used
// for output.
func New(opts ...Option) (*zap.Logger, error) {
	o := defaults()
	c := zap.NewProductionConfig()

	c.OutputPaths = []string{"stdout"}
	c.ErrorOutputPaths = []string{"stdout"}

	for _, opt := range opts {
		opt(o)
	}

	// set sampling
	c.Sampling = &zap.SamplingConfig{
		Initial:    o.SamplingInitial,
		Thereafter: o.SamplingThereafter,
	}

	// logger level
	c.Level = safeLevel(o.Level)
	traceLvl := safeLevel(o.TraceLevel)

	// logger format
	switch f := o.Format; strings.ToLower(f) {
	case formatConsole:
		c.Encoding = formatConsole
	default:
		c.Encoding = formatJSON
	}

	// logger time
	c.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if o.NoCaller {
		c.EncoderConfig.EncodeCaller = nil
	}

	// enable trace only for current log-level
	o.Options = append(o.Options, zap.AddStacktrace(traceLvl))

	l, err := c.Build(o.Options...)
	if err != nil {
		return nil, err
	}

	if o.NoDisclaimer {
		return l, nil
	}

	return l.With(
		zap.String("app_name", o.AppName),
		zap.String("app_version", o.AppVersion)), nil
}
