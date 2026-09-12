//go:build windows

package sysinfo

// Leitura de temperatura via PawnIO (https://pawnio.eu), um driver de kernel
// genérico, de código aberto e assinado pela Microsoft, usado por programas
// como LibreHardwareMonitor (0.9.5+), HWiNFO e Fan Control pra ler registros
// que só existem em modo kernel (MSR na Intel, SMN na AMD — não tem API nem
// WMI pra isso).
//
// O Bifrost NÃO instala esse driver: só tenta abrir \Device\PawnIO, e se um
// desses programas já o deixou instalado, aproveita pra ler a temperatura
// real sem precisar que nenhum deles esteja aberto no momento. Se o driver
// não estiver presente, falha bem baratinho (uma tentativa de abrir arquivo)
// e nenhuma outra coisa acontece — nada é baixado nem instalado.
//
// Os módulos (bytecode compilado, assinado, do repositório oficial
// namazso/PawnIO.Modules) são baixados uma vez e ficados em cache local,
// só quando o driver já está presente.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// --- protocolo PawnIO (ver PawnIOLib em github.com/namazso/PawnIO) --------

const pawnioDeviceType = 41394 // k_device_type, de PawnIO/include/pawnio_um.h

func ctlCode(deviceType, function, method, access uint32) uint32 {
	return (deviceType << 16) | (access << 14) | (function << 2) | method
}

var (
	ioctlPioLoadBinary = ctlCode(pawnioDeviceType, 0x821, 0, 0) // IOCTL_PIO_LOAD_BINARY
	ioctlPioExecuteFn  = ctlCode(pawnioDeviceType, 0x841, 0, 0) // IOCTL_PIO_EXECUTE_FN
)

var (
	modNtdll                  = windows.NewLazySystemDLL("ntdll.dll")
	procNtOpenFile            = modNtdll.NewProc("NtOpenFile")
	procNtDeviceIoControlFile = modNtdll.NewProc("NtDeviceIoControlFile")
)

// unicodeString espelha a UNICODE_STRING do Windows (mesmo layout de
// memória: 2+2 bytes seguidos de padding até o ponteiro de 8 bytes).
type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

// objectAttributes espelha OBJECT_ATTRIBUTES (só os campos que usamos).
type objectAttributes struct {
	Length                   uint32
	RootDirectory            windows.Handle
	ObjectName               *unicodeString
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}

// ioStatusBlock espelha IO_STATUS_BLOCK (união de NTSTATUS/PVOID + ULONG_PTR).
type ioStatusBlock struct {
	Status      int32
	_           int32
	Information uintptr
}

const statusPending = 0x00000103

// openPawnIODevice abre \Device\PawnIO pelo caminho nativo (NtOpenFile), do
// jeito que a própria PawnIOLib faz — não existe mais um symlink em \\.\
// nas versões atuais do driver.
func openPawnIODevice() (windows.Handle, error) {
	path, err := windows.UTF16FromString(`\Device\PawnIO`)
	if err != nil {
		return 0, err
	}
	us := unicodeString{
		Length:        uint16((len(path) - 1) * 2),
		MaximumLength: uint16(len(path) * 2),
		Buffer:        &path[0],
	}
	oa := objectAttributes{ObjectName: &us}
	oa.Length = uint32(unsafe.Sizeof(oa))

	var h windows.Handle
	var iosb ioStatusBlock
	const fileShareDelete = 0x00000004
	r, _, _ := procNtOpenFile.Call(
		uintptr(unsafe.Pointer(&h)),
		uintptr(windows.GENERIC_READ|windows.GENERIC_WRITE),
		uintptr(unsafe.Pointer(&oa)),
		uintptr(unsafe.Pointer(&iosb)),
		uintptr(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|fileShareDelete),
		0,
	)
	if status := int32(uint32(r)); status < 0 {
		return 0, fmt.Errorf("driver PawnIO indisponível (0x%08X)", uint32(status))
	}
	return h, nil
}

// pawnioIoctl manda um IOCTL síncrono pro driver (mesmo padrão da função
// synchronous_ioctl da PawnIOLib: cria um evento, dispara
// NtDeviceIoControlFile, e espera se vier STATUS_PENDING).
func pawnioIoctl(h windows.Handle, code uint32, in []byte, outLen int) ([]byte, error) {
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(ev)

	var inPtr, outPtr unsafe.Pointer
	if len(in) > 0 {
		inPtr = unsafe.Pointer(&in[0])
	}
	out := make([]byte, outLen)
	if outLen > 0 {
		outPtr = unsafe.Pointer(&out[0])
	}

	var iosb ioStatusBlock
	r, _, _ := procNtDeviceIoControlFile.Call(
		uintptr(h), uintptr(ev), 0, 0,
		uintptr(unsafe.Pointer(&iosb)),
		uintptr(code),
		uintptr(inPtr), uintptr(len(in)),
		uintptr(outPtr), uintptr(outLen),
	)
	status := int32(uint32(r))
	if status == statusPending {
		windows.WaitForSingleObject(ev, windows.INFINITE)
		status = iosb.Status
	}
	if status < 0 {
		return nil, fmt.Errorf("ioctl PawnIO falhou (0x%08X)", uint32(status))
	}
	return out[:iosb.Information], nil
}

func pawnioLoad(h windows.Handle, blob []byte) error {
	_, err := pawnioIoctl(h, ioctlPioLoadBinary, blob, 0)
	return err
}

const pawnioFnNameLen = 32

// pawnioExecute chama uma função exportada pelo módulo carregado (ver
// DEFINE_IOCTL_SIZED nos módulos .p): name tem que caber em 31 bytes + nulo.
func pawnioExecute(h windows.Handle, name string, in []uint64, outCount int) ([]uint64, error) {
	if len(name) >= pawnioFnNameLen {
		return nil, fmt.Errorf("nome de função PawnIO muito longo: %q", name)
	}
	buf := make([]byte, pawnioFnNameLen+len(in)*8)
	copy(buf, name)
	for i, v := range in {
		binary.LittleEndian.PutUint64(buf[pawnioFnNameLen+i*8:], v)
	}
	out, err := pawnioIoctl(h, ioctlPioExecuteFn, buf, outCount*8)
	if err != nil {
		return nil, err
	}
	res := make([]uint64, len(out)/8)
	for i := range res {
		res[i] = binary.LittleEndian.Uint64(out[i*8:])
	}
	return res, nil
}

// --- descoberta do fabricante da CPU ---------------------------------------

func cpuVendorID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("VendorIdentifier")
	if err != nil {
		return ""
	}
	return v
}

// --- cache dos módulos (bytecode assinado, baixado uma vez) ----------------

const pawnioModulesRelease = "https://api.github.com/repos/namazso/PawnIO.Modules/releases/latest"

var pawnioHTTPClient = &http.Client{Timeout: 15 * time.Second}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"` // "sha256:<hex>"
}

type githubRelease struct {
	Assets []githubAsset `json:"assets"`
}

func pawnioModuleCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "Bifrost", "pawnio_modules")
	return dir, os.MkdirAll(dir, 0o755)
}

// fetchPawnIOModule devolve os bytes do módulo compilado (ex.: "IntelMSR.bin"),
// do cache local se já tiver, ou baixando o zip da release oficial (com
// conferência de SHA-256 contra o que a própria API do GitHub informa) e
// extraindo só o arquivo pedido.
func fetchPawnIOModule(name string) ([]byte, error) {
	dir, err := pawnioModuleCacheDir()
	if err != nil {
		return nil, err
	}
	cachePath := filepath.Join(dir, name)
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		return data, nil
	}

	req, err := http.NewRequest(http.MethodGet, pawnioModulesRelease, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := pawnioHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	relBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("consultando release do PawnIO.Modules: %w (status %d)", err, resp.StatusCode)
	}
	var rel githubRelease
	if err := json.Unmarshal(relBody, &rel); err != nil {
		return nil, err
	}
	var asset *githubAsset
	for i := range rel.Assets {
		a := &rel.Assets[i]
		if filepath.Ext(a.Name) == ".zip" {
			asset = a
			break
		}
	}
	if asset == nil {
		return nil, fmt.Errorf("nenhum pacote .zip encontrado na release do PawnIO.Modules")
	}

	zipData, err := downloadAndVerify(asset.BrowserDownloadURL, asset.Digest)
	if err != nil {
		return nil, err
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, err
	}
	var result []byte
	for _, f := range zr.File {
		if filepath.Base(f.Name) != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 4<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		// guarda no cache pros próximos módulos pedidos não precisarem baixar de novo
		_ = os.WriteFile(filepath.Join(dir, filepath.Base(f.Name)), data, 0o644)
		if filepath.Base(f.Name) == name {
			result = data
		}
	}
	if result == nil {
		return nil, fmt.Errorf("módulo %q não encontrado no pacote do PawnIO.Modules", name)
	}
	return result, nil
}

func downloadAndVerify(url, digest string) ([]byte, error) {
	resp, err := pawnioHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baixando %s: status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	const prefix = "sha256:"
	if len(digest) > len(prefix) && digest[:len(prefix)] == prefix {
		sum := sha256.Sum256(data)
		want := digest[len(prefix):]
		if hex.EncodeToString(sum[:]) != want {
			return nil, fmt.Errorf("checksum do pacote do PawnIO.Modules não confere")
		}
	}
	return data, nil
}

// --- leitura de temperatura -------------------------------------------------

// Registradores usados (ver PawnIO.Modules: IntelMSR.p / AMDFamily17.p).
const (
	msrIntelTemperatureTarget    = 0x000001a2
	msrIntelPackageThermStatus   = 0x000001b1
	amdSMNAddrTctl               = 0x00059800
	amdSMNTempRangeSelBit uint64 = 1 << 19
)

// readPawnIOIntel lê a temperatura do pacote via MSR (fórmula padrão da
// Intel, documentada no SDM vol. 3, seção "Thermal Monitoring": a leitura é
// "graus abaixo do TjMax", não um valor absoluto.
func readPawnIOIntel(h windows.Handle) (float64, error) {
	blob, err := fetchPawnIOModule("IntelMSR.bin")
	if err != nil {
		return -1, err
	}
	if err := pawnioLoad(h, blob); err != nil {
		return -1, err
	}
	target, err := pawnioExecute(h, "ioctl_read_msr", []uint64{msrIntelTemperatureTarget}, 1)
	if err != nil || len(target) == 0 {
		return -1, err
	}
	tjmax := (target[0] >> 16) & 0xFF
	if tjmax == 0 {
		tjmax = 100 // valor típico quando o registro não informa TjMax
	}
	status, err := pawnioExecute(h, "ioctl_read_msr", []uint64{msrIntelPackageThermStatus}, 1)
	if err != nil || len(status) == 0 {
		return -1, err
	}
	if status[0]&(1<<31) == 0 {
		return -1, fmt.Errorf("leitura de temperatura do pacote inválida")
	}
	readout := (status[0] >> 16) & 0x7F
	return float64(tjmax) - float64(readout), nil
}

// readPawnIOAMD lê "Tctl" via SMN (fórmula do driver k10temp do Linux,
// válida pra toda a família Zen/Zen2/Zen3/Zen4/Zen5).
func readPawnIOAMD(h windows.Handle) (float64, error) {
	blob, err := fetchPawnIOModule("AMDFamily17.bin")
	if err != nil {
		return -1, err
	}
	if err := pawnioLoad(h, blob); err != nil {
		return -1, err
	}
	out, err := pawnioExecute(h, "ioctl_read_smn", []uint64{amdSMNAddrTctl}, 1)
	if err != nil || len(out) == 0 {
		return -1, err
	}
	raw := out[0]
	milliC := float64(raw>>21) * 125.0
	if raw&amdSMNTempRangeSelBit != 0 {
		milliC -= 49000
	}
	return milliC / 1000.0, nil
}

// readPawnIO tenta ler a temperatura da CPU via PawnIO, se (e só se) o
// driver já estiver instalado por outro programa (LibreHardwareMonitor
// 0.9.5+, HWiNFO, Fan Control…). O Bifrost nunca instala esse driver
// sozinho. GPU não é lida por aqui (fica com o nvidia-smi/LHM/etc.).
func readPawnIO() (cpu, gpu float64) {
	h, err := openPawnIODevice()
	if err != nil {
		return -1, -1 // driver não instalado: nada a fazer, sem custo de rede
	}
	defer windows.CloseHandle(h)

	switch cpuVendorID() {
	case "GenuineIntel":
		if t, err := readPawnIOIntel(h); err == nil {
			return t, -1
		}
	case "AuthenticAMD":
		if t, err := readPawnIOAMD(h); err == nil {
			return t, -1
		}
	}
	return -1, -1
}
