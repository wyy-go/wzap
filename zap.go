// Package wzap provides log handling using zap package.
// Code structure based on ginrus package.
package wzap

import (
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type customFieldFunc func(c *gin.Context) zap.Field

type Option func(options *Options)

type skipFunc func(c *gin.Context) bool

// Options is config setting for wzap
type Options struct {
	timeFormat string
	utc        bool
	skipPaths  []string
	logger *zap.Logger
	customFields []customFieldFunc
	stack bool
	skipFunc skipFunc
}

func WithTimeFormat(format string) Option {
	return func(options *Options) {
		options.timeFormat = format
	}
}

func WithUTC(utc bool) Option {
	return func(options *Options) {
		options.utc = utc
	}
}

func WithStack(stack bool) Option {
	return func(options *Options) {
		options.stack = stack
	}
}

func WithSkipPaths(skipPaths ...string) Option {
	return func(options *Options) {
		options.skipPaths = skipPaths
	}
}

func WithZapLogger(logger *zap.Logger) Option {
	return func(options *Options) {
		options.logger = logger
	}
}

func WithCustomFields(fields ...customFieldFunc) Option {
	return func(options *Options) {
		options.customFields = append(options.customFields,fields...)
	}
}

func WithSkip(skip skipFunc) Option {
	return func(options *Options) {
		options.skipFunc = skip
	}
}

func newOptions(opts ...Option) Options {
	options := Options{
		timeFormat: time.RFC3339Nano,
		utc:        false,
		skipPaths:  nil,
		logger:     nil,
		stack:      false,
		skipFunc: 	func(c *gin.Context) bool { return false },
	}

	for _, opt := range opts {
		opt(&options)
	}

	if options.logger == nil {
		panic("zap log err!")
	}

	return options
}

// New returns a gin.HandlerFunc using configs
func New(opts ...Option) gin.HandlerFunc {

	options := newOptions(opts...)


	skipPaths := make(map[string]bool, len(options.skipPaths))
	for _, path := range options.skipPaths {
		skipPaths[path] = true
	}

	return func(c *gin.Context) {
		start := time.Now()
		// some evil middlewares modify this values
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Next()

		if options.skipFunc(c) {
			return
		}

		if _, ok := skipPaths[path]; ok {
			return
		}

		end := time.Now()
		latency := end.Sub(start)
		if options.utc {
			end = end.UTC()
		}

		if len(c.Errors) > 0 {
			// Append error field if this is an erroneous request.
			for _, e := range c.Errors.Errors() {
				options.logger.Error(e)
			}
		} else {
			fields := []zapcore.Field{
				zap.Int("status", c.Writer.Status()),
				zap.String("method", c.Request.Method),
				zap.String("path", path),
				zap.String("query", query),
				zap.String("ip", c.ClientIP()),
				zap.String("user-agent", c.Request.UserAgent()),
				zap.Duration("latency", latency),
				zap.String("time", end.Format(options.timeFormat)),
			}

			for _, field := range options.customFields {
				fields = append(fields, field(c))
			}

			options.logger.Info(path, fields...)
		}
	}
}

// Recovery returns a gin.HandlerFunc (middleware)
// that recovers from any panics and logs requests using uber-go/zap.
// All errors are logged using zap.Error().
// stack means whether output the stack info.
// The stack info is easy to find where the error occurs but the stack info is too large.
func Recovery(opts ...Option) gin.HandlerFunc {
	options := newOptions(opts...)

	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Check for a broken connection, as it is not really a
				// condition that warrants a panic stack trace.
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") ||
							strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				if brokenPipe {
					options.logger.Error(c.Request.URL.Path,
						zap.Any("error", err),
						zap.String("request", string(httpRequest)),
					)
					// If the connection is dead, we can't write a status to it.
					c.Error(err.(error)) // nolint: errcheck
					c.Abort()
					return
				}

				fields := make([]zap.Field,0,len(options.customFields))
				fields = append(fields,
					zap.Time("time", time.Now()),
					zap.Any("error", err),
					zap.String("request", string(httpRequest)))

				for _, field := range options.customFields {
					fields = append(fields, field(c))
				}

				if options.stack {
					fields = append(fields,zap.String("stack", string(debug.Stack())))
				}

				options.logger.Error("[Recovery from panic]", fields...)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
