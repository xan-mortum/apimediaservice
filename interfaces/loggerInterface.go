package interfaces

//интерейс для реализации рекомендации использования интерфейсов и строгой утиной типизации для разрыва зависимостей
//используя этот интерфейс в своих структурах я могу добавлять люой логер который удовлетворяет условия
type Logger interface {
	// Fatal is equivalent to l.Critical followed by a call to os.Exit(1).
	Fatal(args ...interface{})

	// Fatalf is equivalent to l.Critical followed by a call to os.Exit(1).
	Fatalf(format string, args ...interface{})

	// Panic is equivalent to l.Critical(fmt.Sprint()) followed by a call to panic().
	Panic(args ...interface{})

	// Panicf is equivalent to l.Critical followed by a call to panic().
	Panicf(format string, args ...interface{})

	// Critical logs a message using CRITICAL as log level.
	Critical(args ...interface{})

	// Criticalf logs a message using CRITICAL as log level.
	Criticalf(format string, args ...interface{})

	// Error logs a message using ERROR as log level.
	Error(args ...interface{})

	// Errorf logs a message using ERROR as log level.
	Errorf(format string, args ...interface{})

	// Warning logs a message using WARNING as log level.
	Warning(args ...interface{})

	// Warningf logs a message using WARNING as log level.
	Warningf(format string, args ...interface{})

	// Notice logs a message using NOTICE as log level.
	Notice(args ...interface{})

	// Noticef logs a message using NOTICE as log level.
	Noticef(format string, args ...interface{})

	// Info logs a message using INFO as log level.
	Info(args ...interface{})

	// Infof logs a message using INFO as log level.
	Infof(format string, args ...interface{})

	// Debug logs a message using DEBUG as log level.
	Debug(args ...interface{})

	// Debugf logs a message using DEBUG as log level.
	Debugf(format string, args ...interface{})
}
