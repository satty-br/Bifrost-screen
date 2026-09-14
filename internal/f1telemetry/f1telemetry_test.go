package f1telemetry

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func buildHeader(packetID uint8, playerCarIndex uint8) []byte {
	h := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(h[0:2], 2024)
	h[2] = 24                // gameYear
	h[3] = 1                 // gameMajorVersion
	h[4] = 0                 // gameMinorVersion
	h[5] = 1                 // packetVersion
	h[6] = packetID
	binary.LittleEndian.PutUint64(h[7:15], 999)
	binary.LittleEndian.PutUint32(h[15:19], math.Float32bits(12.5))
	binary.LittleEndian.PutUint32(h[19:23], 100)
	binary.LittleEndian.PutUint32(h[23:27], 100)
	h[27] = playerCarIndex
	h[28] = 255
	return h
}

func TestParseCarTelemetry(t *testing.T) {
	const playerIdx = 3
	body := buildHeader(packetIDCarTelemetry, playerIdx)
	body = append(body, make([]byte, maxCars*carTelemetrySize+3)...)

	off := headerSize + playerIdx*carTelemetrySize
	d := body[off : off+carTelemetrySize]
	binary.LittleEndian.PutUint16(d[0:2], 287)                          // speed
	binary.LittleEndian.PutUint32(d[2:6], math.Float32bits(0.95))       // throttle
	binary.LittleEndian.PutUint32(d[10:14], math.Float32bits(0.0))      // brake
	d[15] = byte(int8(7))                                               // gear
	binary.LittleEndian.PutUint16(d[16:18], 11800)                     // engineRPM
	d[18] = 1                                                           // drs on

	s := NovoServer()
	s.handlePacket(body)
	snap := s.Current()

	if snap.Speed != 287 {
		t.Fatalf("Speed = %d, want 287", snap.Speed)
	}
	if snap.Gear != 7 {
		t.Fatalf("Gear = %d, want 7", snap.Gear)
	}
	if snap.EngineRPM != 11800 {
		t.Fatalf("EngineRPM = %d, want 11800", snap.EngineRPM)
	}
	if !snap.DRS {
		t.Fatal("DRS = false, want true")
	}
	if math.Abs(snap.Throttle-0.95) > 0.001 {
		t.Fatalf("Throttle = %v, want ~0.95", snap.Throttle)
	}
	if !snap.Ativa() {
		t.Fatal("Ativa() = false logo após receber pacote")
	}
}

func TestParseLapData(t *testing.T) {
	const playerIdx = 1
	body := buildHeader(packetIDLapData, playerIdx)
	body = append(body, make([]byte, maxCars*lapDataSize+2)...)

	off := headerSize + playerIdx*lapDataSize
	d := body[off : off+lapDataSize]
	binary.LittleEndian.PutUint32(d[0:4], 92345)  // lastLapTimeInMS
	binary.LittleEndian.PutUint32(d[4:8], 45210)  // currentLapTimeInMS
	d[32] = 4                                     // carPosition
	d[33] = 12                                    // currentLapNum
	d[36] = 1                                     // sector (0-based) -> sector 2

	s := NovoServer()
	s.handlePacket(body)
	snap := s.Current()

	if snap.LastLapTimeMS != 92345 {
		t.Fatalf("LastLapTimeMS = %d, want 92345", snap.LastLapTimeMS)
	}
	if snap.CurrentLapTimeMS != 45210 {
		t.Fatalf("CurrentLapTimeMS = %d, want 45210", snap.CurrentLapTimeMS)
	}
	if snap.CarPosition != 4 {
		t.Fatalf("CarPosition = %d, want 4", snap.CarPosition)
	}
	if snap.CurrentLapNum != 12 {
		t.Fatalf("CurrentLapNum = %d, want 12", snap.CurrentLapNum)
	}
	if snap.Sector != 2 {
		t.Fatalf("Sector = %d, want 2", snap.Sector)
	}
}

func TestParseSession(t *testing.T) {
	body := buildHeader(packetIDSession, 0)
	extra := make([]byte, 8)
	extra[3] = 58              // totalLaps
	binary.LittleEndian.PutUint16(extra[4:6], 5303)
	extra[6] = 15              // sessionType -> Corrida
	extra[7] = byte(int8(10))  // trackId -> Spa
	body = append(body, extra...)

	s := NovoServer()
	s.handlePacket(body)
	snap := s.Current()

	if snap.TotalLaps != 58 {
		t.Fatalf("TotalLaps = %d, want 58", snap.TotalLaps)
	}
	if snap.Track != "Spa" {
		t.Fatalf("Track = %q, want Spa", snap.Track)
	}
	if snap.SessionType != "Corrida" {
		t.Fatalf("SessionType = %q, want Corrida", snap.SessionType)
	}
}

func TestSnapshotAtivaExpira(t *testing.T) {
	var s Snapshot
	if s.Ativa() {
		t.Fatal("Snapshot zero não devia estar ativo")
	}
	s.UpdatedAt = time.Now().Add(-(Validade + time.Second))
	if s.Ativa() {
		t.Fatal("Snapshot velho não devia estar ativo")
	}
	s.UpdatedAt = time.Now()
	if !s.Ativa() {
		t.Fatal("Snapshot recente devia estar ativo")
	}
}

func TestHandlePacketIgnoraCurto(t *testing.T) {
	s := NovoServer()
	s.handlePacket([]byte{1, 2, 3})
	if s.Current().Ativa() {
		t.Fatal("pacote curto não devia gerar snapshot ativo")
	}
}
