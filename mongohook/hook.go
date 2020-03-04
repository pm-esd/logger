package mongohook

import (
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/pm-esd/mongodb"
	"github.com/pm-esd/queue"
	"github.com/sirupsen/logrus"
)

var defaultOptions = options{
	maxQueues:  512,
	maxWorkers: 2,
	levels: []logrus.Level{
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
	},
	out: os.Stderr,
}

// FilterHandle 一个过滤器处理程序
type FilterHandle func(*logrus.Entry) *logrus.Entry

type options struct {
	maxQueues  int
	maxWorkers int
	extra      map[string]interface{}
	exec       ExecCloser
	filter     FilterHandle
	levels     []logrus.Level
	out        io.Writer
}

// SetMaxQueues 设置缓冲区的数量
func SetMaxQueues(maxQueues int) Option {
	return func(o *options) {
		o.maxQueues = maxQueues
	}
}

// SetMaxWorkers 设置工作线程数
func SetMaxWorkers(maxWorkers int) Option {
	return func(o *options) {
		o.maxWorkers = maxWorkers
	}
}

// SetExtra 设置扩展参数
func SetExtra(extra map[string]interface{}) Option {
	return func(o *options) {
		o.extra = extra
	}
}

// SetExec 设置Execer接口
func SetExec(exec ExecCloser) Option {
	return func(o *options) {
		o.exec = exec
	}
}

// SetFilter 设置条目过滤器
func SetFilter(filter FilterHandle) Option {
	return func(o *options) {
		o.filter = filter
	}
}

// SetLevels 设置可用的日志级别
func SetLevels(levels ...logrus.Level) Option {
	return func(o *options) {
		if len(levels) == 0 {
			return
		}
		o.levels = levels
	}
}

// SetOut 设置错误输出
func SetOut(out io.Writer) Option {
	return func(o *options) {
		o.out = out
	}
}

// Option 钩子参数选项
type Option func(*options)

// Default create a default mongo hook
func Default(sess *mongodb.MongoDBClient, cName string, opts ...Option) *Hook {
	var options []Option
	options = append(options, opts...)
	options = append(options, SetExec(NewExec(sess, cName)))
	return New(options...)
}

// DefaultWithURL create a default mongo hook
func DefaultWithURL(sess *mongodb.MongoDBClient, cName string, opts ...Option) *Hook {
	var options []Option
	options = append(options, opts...)
	options = append(options, SetExec(NewExecWithURL(sess, cName)))
	return New(options...)
}

// New 创建一个要添加到logger实例的钩子
func New(opt ...Option) *Hook {
	opts := defaultOptions
	for _, o := range opt {
		o(&opts)
	}

	if opts.exec == nil {
		// panic("Unknown Execer interface implementation")
		logrus.Info("Unknown Execer interface implementation")
	}

	q := queue.NewQueue(opts.maxQueues, opts.maxWorkers)
	q.Run()

	return &Hook{
		opts: opts,
		q:    q,
	}
}

// Hook 将日志发送到 mongo 数据库
type Hook struct {
	opts options
	q    *queue.Queue
}

// Levels 返回可用的日志记录级别
func (h *Hook) Levels() []logrus.Level {
	return h.opts.levels
}

// Fire 触发日志事件时将调用
func (h *Hook) Fire(entry *logrus.Entry) error {
	if entry.HasCaller() {
		// funcVal := entry.Caller.Function
		// fileVal := fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)
		// entry.Data["func"] = funcVal
		// entry.Data["file"] = fileVal

		pc := make([]uintptr, 3, 3)
		cnt := runtime.Callers(6, pc)
		for i := 0; i < cnt; i++ {
			fu := runtime.FuncForPC(pc[i] - 1)
			name := fu.Name()
			if !strings.Contains(name, "github.com/sirupsen/logrus") {
				file, line := fu.FileLine(pc[i] - 1)
				fileVal := fmt.Sprintf("%s:%d", file, line)
				entry.Data["file"] = fileVal
				entry.Data["func"] = path.Base(name)

				break
			} else {
				if pc, file, line, ok := runtime.Caller(8); ok {
					funcName := runtime.FuncForPC(pc).Name()
					fileVal := fmt.Sprintf("%s:%d", file, line)
					entry.Data["file"] = fileVal
					entry.Data["func"] = path.Base(funcName)
				}
			}
		}
	}

	entry = h.copyEntry(entry)
	h.q.Push(queue.NewJob(entry, func(v interface{}) {
		h.exec(v.(*logrus.Entry))
	}))
	return nil
}

func (h *Hook) copyEntry(e *logrus.Entry) *logrus.Entry {
	entry := logrus.NewEntry(e.Logger)
	entry.Data = make(logrus.Fields)
	entry.Time = e.Time
	entry.Level = e.Level
	entry.Message = e.Message
	for k, v := range e.Data {
		entry.Data[k] = v
	}
	return entry
}

func (h *Hook) exec(entry *logrus.Entry) {
	if extra := h.opts.extra; extra != nil {
		for k, v := range extra {
			if _, ok := entry.Data[k]; !ok {
				entry.Data[k] = v
			}
		}
	}
	if filter := h.opts.filter; filter != nil {
		entry = filter(entry)
	}
	err := h.opts.exec.Exec(entry)
	if err != nil && h.opts.out != nil {
		fmt.Fprintf(h.opts.out, "[Mongo-Hook] Execution error: %s", err.Error())
	}
}

// Flush 等待日志队列为空
func (h *Hook) Flush() {
	h.q.Terminate()
}
