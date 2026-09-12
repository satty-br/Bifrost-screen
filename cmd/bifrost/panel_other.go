//go:build !windows

package main

import "github.com/satty-br/Bifrost-screen/internal/winutil"

func runPanel(url, dir string) { _ = winutil.OpenURL(url) }
