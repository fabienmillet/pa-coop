//go:build !windows

package main

import (
	"os"
	"syscall"
	"time"
)

// watchParent arrête le serveur quand le jeu qui l'a lancé se termine.
func watchParent(pid int, stop chan os.Signal) {
	for range time.Tick(2 * time.Second) {
		if syscall.Kill(pid, 0) != nil {
			stop <- syscall.SIGTERM
			return
		}
	}
}
