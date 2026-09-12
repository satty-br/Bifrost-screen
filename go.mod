module github.com/satty-br/Bifrost-screen

go 1.25.0

toolchain go1.27.1

require (
	fyne.io/systray v1.12.2
	github.com/fogleman/gg v1.3.0
	github.com/go-ole/go-ole v1.3.0
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	github.com/saltosystems/winrt-go v0.0.0-20260513072510-45f10383b2b8
	github.com/tc-hib/winres v0.3.1
	go.bug.st/serial v1.6.4
	golang.org/x/image v0.41.0
	golang.org/x/sys v0.44.0
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
)

// Os hosts de "vanity import" (golang.org, go.bug.st, fyne.io) são resolvidos
// pelos espelhos oficiais no GitHub — o código é o mesmo, só muda de onde baixa.
replace (
	fyne.io/systray => github.com/fyne-io/systray v1.12.2
	go.bug.st/serial => github.com/bugst/go-serial v1.6.4
	golang.org/x/image => github.com/golang/image v0.41.0
	golang.org/x/sys => github.com/golang/sys v0.44.0
)
