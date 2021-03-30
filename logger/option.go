package logger

import "go.uber.org/zap"

func WithSamplingInitial(v int) Option { return func(o *options) { o.SamplingInitial = v } }

func WithSamplingThereafter(v int) Option { return func(o *options) { o.SamplingThereafter = v } }

func WithFormat(v string) Option { return func(o *options) { o.Format = v } }

func WithLevel(v string) Option { return func(o *options) { o.Level = v } }

func WithTraceLevel(v string) Option { return func(o *options) { o.TraceLevel = v } }

func WithoutDisclaimer() Option { return func(o *options) { o.NoDisclaimer = true } }

func WithoutCaller() Option { return func(o *options) { o.NoCaller = true } }

func WithAppName(v string) Option { return func(o *options) { o.AppName = v } }

func WithAppVersion(v string) Option { return func(o *options) { o.AppVersion = v } }

func WithZapOptions(opts ...zap.Option) Option { return func(o *options) { o.Options = opts } }
