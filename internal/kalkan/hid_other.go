//go:build !windows && !linux

package kalkan

// Devices e OpenPath só existem no Windows e no Linux por enquanto. No macOS
// o acesso HID exigiria o IOKit HID Manager, que precisa de CGO — a mesma
// limitação do mostrador Mancer.

func Devices() ([]DeviceInfo, error) { return nil, ErrUnsupported }

func AllDevices() ([]DeviceInfo, error) { return nil, ErrUnsupported }

func OpenPath(string) (Transport, error) { return nil, ErrUnsupported }
