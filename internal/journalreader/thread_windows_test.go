package journalreader

import "golang.org/x/sys/windows"

func nativeThreadID() uint64 { return uint64(windows.GetCurrentThreadId()) }
