package logger

import "go.uber.org/zap"

// WithSamplingInitial returns Option that sets sampling initial parameter.
func WithSamplingInitial(v int) Option { return func(o *options) { o.SamplingInitial = v } }

// WithSamplingThereafter returns Option that sets sampling thereafter parameter.
func WithSamplingThereafter(v int) Option { return func(o *options) { o.SamplingThereafter = v } }

// WithFormat returns Option that sets format parameter.
func WithFormat(v string) Option { return func(o *options) { o.Format = v } }

// WithLevel returns Option that sets Level parameter.
func WithLevel(v string) Option { return func(o *options) { o.Level = v } }

// WithTraceLevel returns Option that sets trace level parameter.
func WithTraceLevel(v string) Option { return func(o *options) { o.TraceLevel = v } }

// WithoutDisclaimer returns Option that disables disclaimer.
func WithoutDisclaimer() Option { return func(o *options) { o.NoDisclaimer = true } }

// WithoutCaller returns Option that disables caller printing.
func WithoutCaller() Option { return func(o *options) { o.NoCaller = true } }

// WithAppName returns Option that sets application name.
func WithAppName(v string) Option { return func(o *options) { o.AppName = v } }

// WithAppVersion returns Option that sets application version.
func WithAppVersion(v string) Option { return func(o *options) { o.AppVersion = v } }

// WithZapOptions returns Option that sets zap logger options.
func WithZapOptions(opts ...zap.Option) Option { return func(o *options) { o.Options = opts } }
