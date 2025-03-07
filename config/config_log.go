package config

import "go.uber.org/zap"

type LogLevel string

const (
	LogLevelDebug   LogLevel = "debug"
	LogLevelInfo    LogLevel = "info"
	LogLevelWarning LogLevel = "warning"
	LogLevelError   LogLevel = "error"
	LogLevelFatal   LogLevel = "fatal"
	LogLevelPanic   LogLevel = "panic"
)

func (l LogLevel) String() string {
	return string(l)
}

func (l LogLevel) Zap() zap.AtomicLevel {
	switch l {
	case LogLevelDebug:
		return zap.NewAtomicLevelAt(zap.DebugLevel)
	case LogLevelInfo:
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	case LogLevelWarning:
		return zap.NewAtomicLevelAt(zap.WarnLevel)
	case LogLevelError:
		return zap.NewAtomicLevelAt(zap.ErrorLevel)
	case LogLevelFatal:
		return zap.NewAtomicLevelAt(zap.FatalLevel)
	case LogLevelPanic:
		return zap.NewAtomicLevelAt(zap.PanicLevel)
	default:
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	}
}
