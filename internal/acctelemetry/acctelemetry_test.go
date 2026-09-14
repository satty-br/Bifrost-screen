package acctelemetry

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
	"unicode/utf16"
)

func buildPhysics(gas, brake, fuel float32, gear, rpm int32, speedKmh float32) []byte {
	buf := make([]byte, physicsReadSize)
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(gas))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(brake))
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(fuel))
	binary.LittleEndian.PutUint32(buf[16:20], uint32(gear))
	binary.LittleEndian.PutUint32(buf[20:24], uint32(rpm))
	binary.LittleEndian.PutUint32(buf[28:32], math.Float32bits(speedKmh))
	return buf
}

func buildGraphics(status, sessionType, completedLaps, position int32, isInPit bool) []byte {
	buf := make([]byte, graphicsReadSize)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(status))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(sessionType))
	binary.LittleEndian.PutUint32(buf[132:136], uint32(completedLaps))
	binary.LittleEndian.PutUint32(buf[136:140], uint32(position))
	pit := int32(0)
	if isInPit {
		pit = 1
	}
	binary.LittleEndian.PutUint32(buf[160:164], uint32(pit))
	return buf
}

func encodeUTF16(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	buf := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(buf[i*2:i*2+2], v)
	}
	return buf
}

func buildStatic(track string) []byte {
	buf := make([]byte, staticReadSize)
	copy(buf[134:], encodeUTF16(track))
	return buf
}

func TestParseSnapshotLive(t *testing.T) {
	physics := buildPhysics(0.8, 0.0, 55.5, 3, 6500, 187.4)
	graphics := buildGraphics(StatusLive, 2, 5, 4, false)

	snap := parseSnapshot(physics, graphics, "Spa")
	if !snap.Ativa() {
		t.Fatal("Ativa() = false, want true (status LIVE)")
	}
	if snap.Gear != 2 {
		t.Fatalf("Gear = %d, want 2 (raw 3 - 1)", snap.Gear)
	}
	if snap.RPM != 6500 {
		t.Fatalf("RPM = %d, want 6500", snap.RPM)
	}
	if math.Abs(snap.SpeedKmh-187.4) > 0.01 {
		t.Fatalf("SpeedKmh = %v, want ~187.4", snap.SpeedKmh)
	}
	if snap.SessionType != "Corrida" {
		t.Fatalf("SessionType = %q, want Corrida", snap.SessionType)
	}
	if snap.Track != "Spa" {
		t.Fatalf("Track = %q, want Spa", snap.Track)
	}
	if snap.CompletedLaps != 5 || snap.Position != 4 {
		t.Fatalf("CompletedLaps/Position = %d/%d, want 5/4", snap.CompletedLaps, snap.Position)
	}
	if snap.IsInPit {
		t.Fatal("IsInPit = true, want false")
	}
}

func TestParseSnapshotGearReverseAndNeutral(t *testing.T) {
	snap := parseSnapshot(buildPhysics(0, 0, 0, 0, 900, 0), buildGraphics(StatusLive, 0, 0, 0, false), "")
	if snap.Gear != -1 {
		t.Fatalf("Gear = %d, want -1 (reverse)", snap.Gear)
	}

	snap = parseSnapshot(buildPhysics(0, 0, 0, 1, 900, 0), buildGraphics(StatusLive, 0, 0, 0, false), "")
	if snap.Gear != 0 {
		t.Fatalf("Gear = %d, want 0 (neutral)", snap.Gear)
	}
}

func TestParseSnapshotOffNotActive(t *testing.T) {
	snap := parseSnapshot(buildPhysics(0, 0, 0, 1, 0, 0), buildGraphics(StatusOff, -1, 0, 0, false), "")
	if snap.Ativa() {
		t.Fatal("Ativa() = true com status OFF, want false")
	}
}

func TestParseSnapshotBufferCurto(t *testing.T) {
	snap := parseSnapshot(make([]byte, 4), make([]byte, 4), "")
	if snap.Ativa() {
		t.Fatal("buffer curto n\u00e3o devia gerar snapshot ativo")
	}
}

func TestParseTrackName(t *testing.T) {
	name := parseTrackName(buildStatic("Monza"))
	if name != "Monza" {
		t.Fatalf("parseTrackName() = %q, want Monza", name)
	}
}

func TestSnapshotAtivaExpira(t *testing.T) {
	s := Snapshot{Status: StatusLive}
	if s.Ativa() {
		t.Fatal("Snapshot zero (sem UpdatedAt) n\u00e3o devia estar ativo")
	}
	s.UpdatedAt = time.Now().Add(-(Validade + time.Second))
	if s.Ativa() {
		t.Fatal("Snapshot velho n\u00e3o devia estar ativo")
	}
	s.UpdatedAt = time.Now()
	if !s.Ativa() {
		t.Fatal("Snapshot recente devia estar ativo")
	}
}
