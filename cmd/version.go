package cmd

// version is set at build time via -ldflags "-X github.com/ffreis/platform-org/cmd.version=<tag>".
// Falls back to "dev" for local builds.
var version = "dev"
