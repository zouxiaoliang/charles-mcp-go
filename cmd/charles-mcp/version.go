package main

import "runtime/debug"

// Release builds override version with -ldflags "-X main.version=<tag>".
var version = "dev"

func applicationVersion() string {
	info, _ := debug.ReadBuildInfo()
	return resolveVersion(version, info)
}

func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != "" && injected != "dev" {
		return injected
	}
	// go install module@version records the version without linker flags.
	if info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
