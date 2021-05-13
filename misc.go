package main

// Prefix is a prefix used for environment variables containing gateway
// configuration.
const Prefix = "HTTP_GW"

var (
	// Build is a timestamp set during gateway build.
	Build = "now"
	// Version is gateway version.
	Version = "dev"
)
