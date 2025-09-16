// pkg/logger/logger.go
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"synapse-backend/internal/config"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var levelNames = []string{"DEBUG", "INFO", "WARN", "ERROR"}

const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
)

var colorMap = map[LogLevel]string{
	DEBUG: Blue,
	INFO:  Green,
	WARN:  Yellow,
	ERROR: Red,
}

var (
	globalLogger *Logger
	initOnce     sync.Once
)

type Logger struct {
	level LogLevel
	out   io.Writer
	mu    sync.Mutex
	isTTY bool
}

func Init() {
	initOnce.Do(func() {
		level := parseLevel(config.AppConfig.Logger.Level)

		var writers []io.Writer
		stdout := os.Stdout
		writers = append(writers, stdout)
		isTTY := isTerminal(stdout)

		deleteOldLogs()

		// 判断是否启用文件输出
		if config.AppConfig.Logger.Output {
			logDir := config.AppConfig.Logger.OutputDir
			if logDir == "" {
				logDir = "logs" // 默认目录
			}

			// 创建目录
			if err := os.MkdirAll(logDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s [%s] [Logger] 无法创建日志目录 %s: %v\n",
					Red, Reset, time.Now().Format("15:04:05"), logDir, err)
			} else {
				// 按启动时间生成文件名
				timestamp := time.Now().Format("20060102_150405")
				logFile := filepath.Join(logDir, fmt.Sprintf("synapse_%s.log", timestamp))

				f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s[ERROR]%s [%s] [Logger] 无法创建日志文件 %s: %v\n",
						Red, Reset, time.Now().Format("15:04:05"), logFile, err)
				} else {
					writers = append(writers, f)
				}
			}
		}

		var out io.Writer
		if len(writers) == 1 {
			out = writers[0]
		} else {
			out = io.MultiWriter(writers...)
		}

		globalLogger = &Logger{
			level: level,
			out:   out,
			isTTY: isTTY,
		}
	})
}

func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return f.Fd() == os.Stdout.Fd() || f.Fd() == os.Stderr.Fd()
	}
	return false
}

func getCallerPackage() string {
	pc, _, _, ok := runtime.Caller(3)
	if !ok {
		return "unknown"
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}
	name := fn.Name()
	lastSlash := strings.LastIndex(name, "/")
	if lastSlash == -1 {
		return name
	}
	start := lastSlash + 1
	end := strings.Index(name[start:], ".")
	if end == -1 {
		return name[start:]
	}
	return name[start : start+end]
}

func (l *Logger) output(level LogLevel, message string) {
	if level < l.level {
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	pkg := getCallerPackage()
	levelStr := levelNames[level]

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.isTTY {
		color := colorMap[level]
		fmt.Fprintf(l.out, "%s[%-5s]%s [%s] [%s] %s\n",
			color, levelStr, Reset, now, pkg, message)
	} else {
		fmt.Fprintf(l.out, "[%s] [%s] [%s] %s\n",
			levelStr, now, pkg, message)
	}
}

func ensureInit() {
	if globalLogger == nil {
		Init()
	}
}

func parseLevel(levelStr string) LogLevel {
	switch strings.ToLower(levelStr) {
	case "debug":
		return DEBUG
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	default:
		return INFO
	}
}

var logFilePattern = regexp.MustCompile(`^synapse_\d{8}_\d{6}\.log$`)

func deleteOldLogs() {
	logDir := config.AppConfig.Logger.OutputDir
	if logDir == "" {
		logDir = "logs"
	}

	files, err := os.ReadDir(logDir)
	if err != nil {
		// 在初始化期间不能使用logger，直接输出到stderr
		fmt.Fprintf(os.Stderr, "无法读取日志目录 %s: %v\n", logDir, err)
		return
	}

	var logFiles []struct {
		name string
		time time.Time
	}

	for _, file := range files {
		if !file.IsDir() && logFilePattern.MatchString(file.Name()) {
			fi, err := file.Info()
			if err != nil {
				// 在初始化期间不能使用logger，直接输出到stderr
				fmt.Fprintf(os.Stderr, "无法获取文件信息 %s: %v\n", file.Name(), err)
				continue
			}
			logFiles = append(logFiles, struct {
				name string
				time time.Time
			}{name: file.Name(), time: fi.ModTime()})
		}
	}

	// 按修改时间降序：最新在前
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].time.After(logFiles[j].time)
	})

	// 删除超出50个的旧文件
	if len(logFiles) > 50 {
		for _, f := range logFiles[50:] {
			path := filepath.Join(logDir, f.name)
			if err := os.Remove(path); err != nil {
				// 在初始化期间不能使用logger，直接输出到stderr
				fmt.Fprintf(os.Stderr, "无法删除旧日志文件 %s: %v\n", f.name, err)
			} else {
				// 在初始化期间不能使用logger，直接输出到stderr
				fmt.Fprintf(os.Stderr, "已删除旧日志文件: %s\n", f.name)
			}
		}
	}
}

// ========== 导出 API ==========

func Info(message string) { ensureInit(); globalLogger.output(INFO, message) }
func Infof(format string, a ...interface{}) {
	Info(fmt.Sprintf(format, a...))
}

func Debug(message string) { ensureInit(); globalLogger.output(DEBUG, message) }
func Debugf(format string, a ...interface{}) {
	Debug(fmt.Sprintf(format, a...))
}

func Warn(message string) { ensureInit(); globalLogger.output(WARN, message) }
func Warnf(format string, a ...interface{}) {
	Warn(fmt.Sprintf(format, a...))
}

func Error(message string) { ensureInit(); globalLogger.output(ERROR, message) }
func Errorf(format string, a ...interface{}) {
	Error(fmt.Sprintf(format, a...))
}
