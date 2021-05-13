package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/grpclog"
)

type (
	zapLogger struct {
		zapcore.Core
		log *zap.SugaredLogger
	}

	// Logger includes grpclog.LoggerV2 interface with an additional
	// Println method.
	Logger interface {
		grpclog.LoggerV2
		Println(v ...interface{})
	}
)

// GRPC wraps given zap.Logger into grpclog.LoggerV2+ interface.
func GRPC(l *zap.Logger) Logger {
	log := l.WithOptions(
		// skip gRPCLog + zapLogger in caller
		zap.AddCallerSkip(2))

	return &zapLogger{
		Core: log.Core(),
		log:  log.Sugar(),
	}
}

// Info implements grpclog.LoggerV2.
func (z *zapLogger) Info(args ...interface{}) { z.log.Info(args...) }

// Infoln implements grpclog.LoggerV2.
func (z *zapLogger) Infoln(args ...interface{}) { z.log.Info(args...) }

// Infof implements grpclog.LoggerV2.
func (z *zapLogger) Infof(format string, args ...interface{}) { z.log.Infof(format, args...) }

// Println allows to print a line with info severity.
func (z *zapLogger) Println(args ...interface{}) { z.log.Info(args...) }

// Printf implements grpclog.LoggerV2.
func (z *zapLogger) Printf(format string, args ...interface{}) { z.log.Infof(format, args...) }

// Warning implements grpclog.LoggerV2.
func (z *zapLogger) Warning(args ...interface{}) { z.log.Warn(args...) }

// Warningln implements grpclog.LoggerV2.
func (z *zapLogger) Warningln(args ...interface{}) { z.log.Warn(args...) }

// Warningf implements grpclog.LoggerV2.
func (z *zapLogger) Warningf(format string, args ...interface{}) { z.log.Warnf(format, args...) }

// Error implements grpclog.LoggerV2.
func (z *zapLogger) Error(args ...interface{}) { z.log.Error(args...) }

// Errorln implements grpclog.LoggerV2.
func (z *zapLogger) Errorln(args ...interface{}) { z.log.Error(args...) }

// Errorf implements grpclog.LoggerV2.
func (z *zapLogger) Errorf(format string, args ...interface{}) { z.log.Errorf(format, args...) }

// Fatal implements grpclog.LoggerV2.
func (z *zapLogger) Fatal(args ...interface{}) { z.log.Fatal(args...) }

// Fatalln implements grpclog.LoggerV2.
func (z *zapLogger) Fatalln(args ...interface{}) { z.log.Fatal(args...) }

// Fatalf implements grpclog.LoggerV2.
func (z *zapLogger) Fatalf(format string, args ...interface{}) { z.log.Fatalf(format, args...) }

// V implements grpclog.LoggerV2.
func (z *zapLogger) V(int) bool { return z.Enabled(zapcore.DebugLevel) }
