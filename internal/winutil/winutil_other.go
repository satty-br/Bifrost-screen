//go:build !windows

package winutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func ListProcesses() ([]Process, error) { return nil, nil }
func KillElevated([]uint32) error       { return errors.New("só no Windows") }
func OpenURL(url string) error          { return exec.Command("xdg-open", url).Start() }
func SetAutostart(bool) error           { return nil }
func AutostartEnabled() bool            { return false }
func SingleInstance() bool              { return true }
func Alert(title, msg string)           { fmt.Fprintln(os.Stderr, title+": "+msg) }

func FocusWindow(string) bool { return false }
func NamedMutex(string) bool  { return true }
