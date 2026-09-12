//go:build !windows

package main

// Fora do Windows a temperatura vem do próprio sistema (hwmon no Linux, SMC no
// macOS), então não há agente nem driver para instalar.

func runSensorAgent()             {}
func runSensorSetup(remover bool) {}
