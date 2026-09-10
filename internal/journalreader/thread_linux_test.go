package journalreader

import "golang.org/x/sys/unix"

func nativeThreadID() uint64 { return uint64(unix.Gettid()) }
