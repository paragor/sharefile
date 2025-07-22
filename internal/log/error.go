package log

import "log/slog"

func Error(err error) slog.Attr {
	value := ""
	if err != nil {
		value = err.Error()
	}
	return slog.Attr{
		Key:   "err",
		Value: slog.StringValue(value),
	}
}
