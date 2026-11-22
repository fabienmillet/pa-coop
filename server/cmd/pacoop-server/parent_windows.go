//go:build windows

package main

import (
	"os"
	"syscall"
)

// watchParent arrête le serveur quand le jeu qui l'a lancé se termine.
func watchParent(pid int, stop chan os.Signal) {
	const synchronize = 0x00100000
	h, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return
	}
	syscall.WaitForSingleObject(h, syscall.INFINITE)
	stop <- os.Interrupt
}
