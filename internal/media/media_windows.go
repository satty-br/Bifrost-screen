//go:build windows

package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/media/control"
	"github.com/saltosystems/winrt-go/windows/storage/streams"
)

const (
	iidIRandomAccessStream = "905A0FE1-BC53-11DF-8C49-001E4FC686DA"
	iidIInputStream        = "905A0FE2-BC53-11DF-8C49-001E4FC686DA"
	inputStreamReadAhead   = 2
	asyncTimeout           = 3 * time.Second
	maxCoverBytes          = 8 << 20
)

// Start inicia a leitura periódica numa thread própria (exigência do WinRT/COM).
func (r *Reader) Start(interval time.Duration) {
	r.backend = "winrt"
	go func() {
		runtime.LockOSThread()
		if err := ole.RoInitialize(1); err != nil { // multithreaded
			log.Printf("mídia: RoInitialize falhou: %v", err)
		}
		var mgr *control.GlobalSystemMediaTransportControlsSessionManager
		var lastCoverKey string
		var lastCover image.Image
		for {
			info, newMgr, err := poll(mgr, lastCoverKey, lastCover)
			mgr = newMgr
			if err == nil && info.HasSession {
				key := info.Title + "\x00" + info.Artist + "\x00" + info.Album
				if info.Cover != nil {
					lastCoverKey, lastCover = key, info.Cover
				}
			}
			r.set(info, err)
			time.Sleep(interval)
		}
	}()
}

func poll(mgr *control.GlobalSystemMediaTransportControlsSessionManager, lastKey string, lastCover image.Image) (info Info, outMgr *control.GlobalSystemMediaTransportControlsSessionManager, err error) {
	outMgr = mgr
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("falha no WinRT: %v", rec)
			outMgr = nil // força pedir um gerenciador novo na próxima vez
		}
	}()

	if outMgr == nil {
		op, e := control.GlobalSystemMediaTransportControlsSessionManagerRequestAsync()
		if e != nil {
			return Info{}, nil, e
		}
		res, e := awaitOperation(op)
		if e != nil {
			return Info{}, nil, e
		}
		outMgr = (*control.GlobalSystemMediaTransportControlsSessionManager)(res)
	}

	sess, e := outMgr.GetCurrentSession()
	if e != nil {
		return Info{}, nil, e
	}
	if sess == nil {
		return Info{}, outMgr, nil
	}
	defer sess.Release()

	info.HasSession = true
	info.UpdatedAt = time.Now()
	if app, e := sess.GetSourceAppUserModelId(); e == nil {
		info.App = friendlyApp(app)
	}

	if op, e := sess.TryGetMediaPropertiesAsync(); e == nil {
		if res, e := awaitOperation(op); e == nil && res != nil {
			props := (*control.GlobalSystemMediaTransportControlsSessionMediaProperties)(res)
			info.Title, _ = props.GetTitle()
			info.Artist, _ = props.GetArtist()
			info.Album, _ = props.GetAlbumTitle()
			info.Title = strings.TrimSpace(info.Title)
			info.Artist = strings.TrimSpace(info.Artist)
			info.Album = strings.TrimSpace(info.Album)
			key := info.Title + "\x00" + info.Artist + "\x00" + info.Album
			if key == lastKey && lastCover != nil {
				info.Cover = lastCover
			} else if thumb, e := props.GetThumbnail(); e == nil && thumb != nil {
				info.Cover = readThumbnail(thumb)
				thumb.Release()
			}
			props.Release()
		}
	}

	if pb, e := sess.GetPlaybackInfo(); e == nil && pb != nil {
		st, _ := pb.GetPlaybackStatus()
		info.Playing = st == control.GlobalSystemMediaTransportControlsSessionPlaybackStatusPlaying
		pb.Release()
	}

	if tl, e := sess.GetTimelineProperties(); e == nil && tl != nil {
		if p, e := tl.GetPosition(); e == nil {
			info.Position = time.Duration(p.Duration * 100)
		}
		if end, e := tl.GetEndTime(); e == nil {
			info.Duration = time.Duration(end.Duration * 100)
		}
		tl.Release()
	}
	return info, outMgr, nil
}

// awaitOperation espera um IAsyncOperation terminar consultando o IAsyncInfo.
func awaitOperation(op *foundation.IAsyncOperation) (unsafe.Pointer, error) {
	if op == nil {
		return nil, errors.New("operação nula")
	}
	defer op.Release()
	if err := waitAsync(&op.IInspectable.IUnknown); err != nil {
		return nil, err
	}
	return op.GetResults()
}

func waitAsync(unk *ole.IUnknown) error {
	itf, err := unk.QueryInterface(ole.NewGUID(foundation.GUIDIAsyncInfo))
	if err != nil {
		return err
	}
	info := (*foundation.IAsyncInfo)(unsafe.Pointer(itf))
	defer info.Release()
	deadline := time.Now().Add(asyncTimeout)
	for {
		st, err := info.GetStatus()
		if err != nil {
			return err
		}
		switch st {
		case foundation.AsyncStatusCompleted:
			return nil
		case foundation.AsyncStatusCanceled:
			return errors.New("operação cancelada")
		case foundation.AsyncStatusError:
			code, _ := info.GetErrorCode()
			return fmt.Errorf("operação falhou (0x%08x)", uint32(code.Value))
		}
		if time.Now().After(deadline) {
			_ = info.Cancel()
			return errors.New("tempo esgotado esperando o Windows")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// readThumbnail lê a capa do álbum (IRandomAccessStreamReference -> imagem).
func readThumbnail(ref *streams.IRandomAccessStreamReference) (img image.Image) {
	defer func() {
		if rec := recover(); rec != nil {
			img = nil
		}
	}()
	op, err := ref.OpenReadAsync()
	if err != nil {
		return nil
	}
	res, err := awaitOperation(op)
	if err != nil || res == nil {
		return nil
	}
	stream := (*ole.IUnknown)(res)
	defer stream.Release()

	// Tamanho: IRandomAccessStream::get_Size (vtable 6).
	ras, err := stream.QueryInterface(ole.NewGUID(iidIRandomAccessStream))
	if err != nil {
		return nil
	}
	var size uint64
	hr, _, _ := syscall.SyscallN(vtbl(unsafe.Pointer(ras), 6), uintptr(unsafe.Pointer(ras)), uintptr(unsafe.Pointer(&size)))
	ras.Release()
	if hr != 0 || size == 0 || size > maxCoverBytes {
		return nil
	}

	buf, err := streams.BufferCreate(uint32(size))
	if err != nil {
		return nil
	}
	defer buf.Release()
	ibufUnk, err := buf.QueryInterface(ole.NewGUID(streams.GUIDIBuffer))
	if err != nil {
		return nil
	}
	defer ibufUnk.Release()

	// IInputStream::ReadAsync(buffer, count, options, out op) (vtable 6).
	in, err := stream.QueryInterface(ole.NewGUID(iidIInputStream))
	if err != nil {
		return nil
	}
	defer in.Release()
	var readOp *ole.IUnknown
	hr, _, _ = syscall.SyscallN(vtbl(unsafe.Pointer(in), 6), uintptr(unsafe.Pointer(in)),
		uintptr(unsafe.Pointer(ibufUnk)), uintptr(uint32(size)), uintptr(inputStreamReadAhead),
		uintptr(unsafe.Pointer(&readOp)))
	if hr != 0 || readOp == nil {
		return nil
	}
	defer readOp.Release()
	if err := waitAsync(readOp); err != nil {
		return nil
	}
	// IAsyncOperationWithProgress<IBuffer,UInt32>::GetResults (vtable 10).
	var outBuf *streams.IBuffer
	hr, _, _ = syscall.SyscallN(vtbl(unsafe.Pointer(readOp), 10), uintptr(unsafe.Pointer(readOp)), uintptr(unsafe.Pointer(&outBuf)))
	if hr != 0 || outBuf == nil {
		return nil
	}
	defer outBuf.Release()
	n, err := outBuf.GetLength()
	if err != nil || n == 0 {
		return nil
	}
	reader, err := streams.DataReaderFromBuffer(outBuf)
	if err != nil || reader == nil {
		return nil
	}
	defer reader.Release()
	data, err := reader.ReadBytes(n)
	if err != nil {
		return nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return decoded
}

// vtbl devolve o endereço do método index da vtable do objeto COM obj.
func vtbl(obj unsafe.Pointer, index int) uintptr {
	table := *(*unsafe.Pointer)(obj)
	return *(*uintptr)(unsafe.Add(table, uintptr(index)*unsafe.Sizeof(uintptr(0))))
}

func friendlyApp(id string) string {
	l := strings.ToLower(id)
	switch {
	case strings.Contains(l, "spotify"):
		return "Spotify"
	case strings.Contains(l, "chrome"):
		return "Chrome"
	case strings.Contains(l, "msedge"):
		return "Edge"
	case strings.Contains(l, "firefox"):
		return "Firefox"
	case strings.Contains(l, "zunemusic") || strings.Contains(l, "media player"):
		return "Media Player"
	case strings.Contains(l, "vlc"):
		return "VLC"
	case strings.Contains(l, "deezer"):
		return "Deezer"
	case strings.Contains(l, "tidal"):
		return "TIDAL"
	case strings.Contains(l, "discord"):
		return "Discord"
	}
	if i := strings.LastIndexAny(id, `\/!`); i >= 0 && i+1 < len(id) {
		id = id[i+1:]
	}
	return strings.TrimSuffix(id, ".exe")
}
