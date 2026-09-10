//go:build windows && (amd64 || arm64)

// This test-only publisher writes fixed metadata-only events to channels owned
// by the native fixture manifest. It is never built into an observer artifact.
package main

import (
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const providerID = "{4F0A8EAD-523C-4F1D-9D5E-30D03A30F81B}"

type eventDescriptor struct {
	ID      uint16
	Version uint8
	Channel uint8
	Level   uint8
	Opcode  uint8
	Task    uint16
	Keyword uint64
}

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "before" && os.Args[1] != "after") {
		os.Exit(2)
	}
	guid, err := windows.GUIDFromString(providerID)
	if err != nil {
		os.Exit(1)
	}
	dll := windows.NewLazySystemDLL("advapi32.dll")
	register := dll.NewProc("EventRegister")
	writeEvent := dll.NewProc("EventWrite")
	unregister := dll.NewProc("EventUnregister")
	for _, procedure := range []*windows.LazyProc{register, writeEvent, unregister} {
		if procedure.Find() != nil {
			os.Exit(1)
		}
	}
	var registration uint64
	result, _, _ := register.Call(uintptr(unsafe.Pointer(&guid)), 0, 0, uintptr(unsafe.Pointer(&registration)))
	runtime.KeepAlive(guid)
	if result != 0 || registration == 0 {
		os.Exit(1)
	}
	exit := 0
	for _, descriptor := range descriptors(os.Args[1]) {
		result, _, _ = writeEvent.Call(uintptr(registration), uintptr(unsafe.Pointer(&descriptor)), 0, 0)
		runtime.KeepAlive(descriptor)
		if result != 0 {
			exit = 1
			break
		}
	}
	result, _, _ = unregister.Call(uintptr(registration))
	if result != 0 {
		exit = 1
	}
	os.Exit(exit)
}

func descriptors(phase string) []eventDescriptor {
	if phase == "before" {
		return []eventDescriptor{
			{ID: 101, Channel: 16, Level: 4},
			{ID: 102, Channel: 16, Level: 3},
			{ID: 201, Channel: 17, Level: 2},
			{ID: 202, Channel: 17, Level: 1},
		}
	}
	return []eventDescriptor{
		{ID: 103, Channel: 16, Level: 2},
		{ID: 203, Channel: 17, Level: 4},
	}
}
