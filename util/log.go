package util

// 参考
// https://github.com/siddontang/go-log/blob/master/log/log.go
//
import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"time"
)

// levels
const (
	DebugLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

//ErrUnknownLevel 定义错误
var ErrUnknownLevel = errors.New("Logger.NewLogger|未知级别")

//LevelName 等级名称
var LevelName = []string{"[Debug]", "[Info ]", "[Warn ]", "[Error]", "[Fatal]"}

//Logger 日志
type Logger struct {
	level      int
	baseLogger *log.Logger
	baseFile   *os.File
	Mark       string
}

//NewLogger 新日志
func NewLogger(level int, logPath string) (*Logger, error) {
	// level
	if level > FatalLevel {
		return nil, ErrUnknownLevel
	}
	// logger
	var baseLogger *log.Logger
	var baseFile *os.File
	if logPath != "" {
		now := time.Now()
		filename := fmt.Sprintf("%d%02d%02d_%02d_%02d_%02d.log",
			now.Year(),
			now.Month(),
			now.Day(),
			now.Hour(),
			now.Minute(),
			now.Second())
		file, err := os.Create(path.Join(logPath, filename))
		if err != nil {
			return nil, err
		}
		baseLogger = log.New(file, "", log.Ldate|log.Ltime)
		baseFile = file
	} else {
		baseLogger = log.New(os.Stdout, "", log.Ldate|log.Ltime)
	}
	// new
	logger := &Logger{
		level:      level,
		baseLogger: baseLogger,
		baseFile:   baseFile,
		Mark:       "",
	}
	return logger, nil
}

//SetLevel 设置等级
func (logger *Logger) SetLevel(l int) {
	logger.level = l
}

//Close It's dangerous to call the method on logging
func (logger *Logger) Close() {
	if logger.baseFile != nil {
		logger.baseFile.Close()
	}
	logger.baseLogger = nil
	logger.baseFile = nil
}

//doPrint 输出
func (logger *Logger) doPrint(level int, a ...interface{}) {
	if level < logger.level {
		return
	}
	if logger.baseLogger == nil {
		panic("Logger.doPrint|logger.baseLogger == nil，日志异常关闭")
	}
	logger.baseLogger.SetPrefix(LevelName[level])
	s := fmt.Sprint(a...)
	logger.baseLogger.Println(logger.Mark + s)
	//	logger.baseLogger.Println(a...)
	if level == FatalLevel {
		logger.Close()
		os.Exit(1)
	}
}

//SetMark 设置消息前缀
func (logger *Logger) SetMark(m string) {
	logger.Mark = logger.Mark + m + "."
}

//Debug Debug
func (logger *Logger) Debug(a ...interface{}) {
	logger.doPrint(DebugLevel, a...)
}

//Info Info
func (logger *Logger) Info(a ...interface{}) {
	logger.doPrint(InfoLevel, a...)
}

//Warn Warn
func (logger *Logger) Warn(a ...interface{}) {
	logger.doPrint(WarnLevel, a...)
}

//Error Error
func (logger *Logger) Error(a ...interface{}) {
	logger.doPrint(ErrorLevel, a...)
}

//Fatal Fatal
func (logger *Logger) Fatal(a ...interface{}) {
	logger.doPrint(FatalLevel, a...)
}
