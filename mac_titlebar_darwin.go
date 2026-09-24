//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa
void livetoolSetOpaqueMacTitlebar(void *window);
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

func setOpaqueMacTitlebar(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	if nativeWindow := window.NativeWindow(); nativeWindow != nil {
		C.livetoolSetOpaqueMacTitlebar(nativeWindow)
	}
}
