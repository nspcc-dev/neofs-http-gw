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

	Logger interface {
		grpclog.LoggerV2
		Println(v ...interface{})
	}
)

func GRPC(l *zap.Logger) Logger {
	log := l.WithOptions(
		// skip gRPCLog + zapLogger in caller
		zap.AddCallerSkip(2))

	return &zapLogger{
		Core: log.Core(),
		log:  log.Sugar(),
	}
}

func (z *zapLogger) Info(args ...interface{}) { z.log.Info(args...) }

func (z *zapLogger) Infoln(args ...interface{}) { z.log.Info(args...) }

func (z *zapLogger) Infof(format string, args ...interface{}) { z.log.Infof(format, args...) }

func (z *zapLogger) Println(args ...interface{}) { z.log.Info(args...) }

func (z *zapLogger) Printf(format string, args ...interface{}) { z.log.Infof(format, args...) }

func (z *zapLogger) Warning(args ...interface{}) { z.log.Warn(args...) }

func (z *zapLogger) Warningln(args ...interface{}) { z.log.Warn(args...) }

func (z *zapLogger) Warningf(format string, args ...interface{}) { z.log.Warnf(format, args...) }

func (z *zapLogger) Error(args ...interface{}) { z.log.Error(args...) }

func (z *zapLogger) Errorln(args ...interface{}) { z.log.Error(args...) }

func (z *zapLogger) Errorf(format string, args ...interface{}) { z.log.Errorf(format, args...) }

func (z *zapLogger) Fatal(args ...interface{}) { z.log.Fatal(args...) }

func (z *zapLogger) Fatalln(args ...interface{}) { z.log.Fatal(args...) }

func (z *zapLogger) Fatalf(format string, args ...interface{}) { z.Fatalf(format, args...) }

func (z *zapLogger) V(int) bool { return z.Enabled(zapcore.DebugLevel) }
