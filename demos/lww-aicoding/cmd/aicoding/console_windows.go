//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

func configureConsoleEncoding() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	const utf8CodePage = 65001
	const enableVirtualTerminalProcessing = 0x0004
	const stdOutputHandle = ^uint32(10)

	_, _, _ = setConsoleCP.Call(uintptr(utf8CodePage))
	_, _, _ = setConsoleOutputCP.Call(uintptr(utf8CodePage))

	handle, _, _ := getStdHandle.Call(uintptr(stdOutputHandle))
	if handle == 0 || handle == uintptr(syscall.InvalidHandle) {
		return
	}

	var mode uint32
	result, _, _ := getConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if result == 0 {
		return
	}
	_, _, _ = setConsoleMode.Call(handle, uintptr(mode|enableVirtualTerminalProcessing))
}
