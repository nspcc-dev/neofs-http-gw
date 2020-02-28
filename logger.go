package main

import (
	"strings"

	"google.golang.org/grpc/grpclog"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type zapLogger struct {
	log *zap.Logger
}

const (
	formatJSON    = "json"
	formatConsole = "console"

	defaultSamplingInitial    = 100
	defaultSamplingThereafter = 100
)

func gRPCLogger(l *zap.Logger) grpclog.LoggerV2 {
	return &zapLogger{
		log: l.WithOptions(
			// skip gRPCLog + zapLogger in caller
			zap.AddCallerSkip(2)),
	}
}

func safeLevel(lvl string) zap.AtomicLevel {
	switch strings.ToLower(lvl) {
	case "debug":
		return zap.NewAtomicLevelAt(zap.DebugLevel)
	case "warn":
		return zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		return zap.NewAtomicLevelAt(zap.ErrorLevel)
	case "fatal":
		return zap.NewAtomicLevelAt(zap.FatalLevel)
	case "panic":
		return zap.NewAtomicLevelAt(zap.PanicLevel)
	default:
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	}
}

func newLogger(v *viper.Viper) *zap.Logger {
	c := zap.NewProductionConfig()

	c.OutputPaths = []string{"stdout"}
	c.ErrorOutputPaths = []string{"stdout"}

	if v.IsSet("logger.sampling") {
		c.Sampling = &zap.SamplingConfig{
			Initial:    defaultSamplingInitial,
			Thereafter: defaultSamplingThereafter,
		}

		if val := v.GetInt("logger.sampling.initial"); val > 0 {
			c.Sampling.Initial = val
		}

		if val := v.GetInt("logger.sampling.thereafter"); val > 0 {
			c.Sampling.Thereafter = val
		}
	}

	// logger level
	c.Level = safeLevel(v.GetString("logger.level"))
	traceLvl := safeLevel(v.GetString("logger.trace_level"))

	// logger format
	switch f := v.GetString("logger.format"); strings.ToLower(f) {
	case formatConsole:
		c.Encoding = formatConsole
	default:
		c.Encoding = formatJSON
	}

	// logger time
	c.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	l, err := c.Build(
		// enable trace only for current log-level
		zap.AddStacktrace(traceLvl))
	if err != nil {
		panic(err)
	}

	if v.GetBool("logger.no_disclaimer") {
		return l
	}

	name := v.GetString("app.name")
	version := v.GetString("app.version")

	return l.With(
		zap.String("app_name", name),
		zap.String("app_version", version))
}

func (z *zapLogger) Info(args ...interface{}) {
	z.log.Sugar().Info(args...)
}

func (z *zapLogger) Infoln(args ...interface{}) {
	z.log.Sugar().Info(args...)
}

func (z *zapLogger) Infof(format string, args ...interface{}) {
	z.log.Sugar().Infof(format, args...)
}

func (z *zapLogger) Println(args ...interface{}) {
	z.log.Sugar().Info(args...)
}

func (z *zapLogger) Printf(format string, args ...interface{}) {
	z.log.Sugar().Infof(format, args...)
}

func (z *zapLogger) Warning(args ...interface{}) {
	z.log.Sugar().Warn(args...)
}

func (z *zapLogger) Warningln(args ...interface{}) {
	z.log.Sugar().Warn(args...)
}

func (z *zapLogger) Warningf(format string, args ...interface{}) {
	z.log.Sugar().Warnf(format, args...)
}

func (z *zapLogger) Error(args ...interface{}) {
	z.log.Sugar().Error(args...)
}

func (z *zapLogger) Errorln(args ...interface{}) {
	z.log.Sugar().Error(args...)
}

func (z *zapLogger) Errorf(format string, args ...interface{}) {
	z.log.Sugar().Errorf(format, args...)
}

func (z *zapLogger) Fatal(args ...interface{}) {
	z.log.Sugar().Fatal(args...)
}

func (z *zapLogger) Fatalln(args ...interface{}) {
	z.log.Sugar().Fatal(args...)
}

func (z *zapLogger) Fatalf(format string, args ...interface{}) {
	z.log.Sugar().Fatalf(format, args...)
}

func (z *zapLogger) V(int) bool { return z.log.Core().Enabled(zapcore.DebugLevel) }
