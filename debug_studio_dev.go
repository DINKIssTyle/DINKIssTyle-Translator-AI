//go:build !production

package main

import "os"

func isDebugStudioRequested() bool {
	return len(os.Args) > 1 && os.Args[1] == "--debug-studio-window"
}
