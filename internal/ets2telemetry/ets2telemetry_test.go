package ets2telemetry

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func buildBuffer() []byte {
	buf := make([]byte, MMFSize)
	buf[offSDKActive] = 1
	binary.LittleEndian.PutUint32(buf[offGameID:offGameID+4], 2) // ATS
	binary.LittleEndian.PutUint32(buf[offGearDash:offGearDash+4], uint32(int32(6)))
	binary.LittleEndian.PutUint32(buf[offFuelCapCfg:offFuelCapCfg+4], math.Float32bits(400))
	binary.LittleEndian.PutUint32(buf[offSpeed:offSpeed+4], math.Float32bits(25)) // 25 m/s ~ 90 km/h
	binary.LittleEndian.PutUint32(buf[offEngineRPM:offEngineRPM+4], math.Float32bits(1500))
	binary.LittleEndian.PutUint32(buf[offFuel:offFuel+4], math.Float32bits(200))
	binary.LittleEndian.PutUint32(buf[offFuelRange:offFuelRange+4], math.Float32bits(650))
	binary.LittleEndian.PutUint32(buf[offSpeedLimit:offSpeedLimit+4], math.Float32bits(0))
	return buf
}

func TestParseSnapshotATS(t *testing.T) {
	snap, ok := parseSnapshot(buildBuffer())
	if !ok {
		t.Fatal("parseSnapshot() ok = false, want true")
	}
	if snap.Game != "American Truck Simulator" {
		t.Fatalf("Game = %q, want American Truck Simulator", snap.Game)
	}
	if snap.Gear != 6 {
		t.Fatalf("Gear = %d, want 6", snap.Gear)
	}
	if math.Abs(snap.SpeedKMH-90) > 0.01 {
		t.Fatalf("SpeedKMH = %v, want ~90", snap.SpeedKMH)
	}
	if snap.EngineRPM != 1500 {
		t.Fatalf("EngineRPM = %d, want 1500", snap.EngineRPM)
	}
	if snap.FuelLiters != 200 {
		t.Fatalf("FuelLiters = %v, want 200", snap.FuelLiters)
	}
	if math.Abs(snap.FuelPct-50) > 0.01 {
		t.Fatalf("FuelPct = %v, want 50", snap.FuelPct)
	}
	if snap.FuelRangeKM != 650 {
		t.Fatalf("FuelRangeKM = %v, want 650", snap.FuelRangeKM)
	}
	if !snap.Ativa() {
		t.Fatal("Ativa() = false logo após parsear")
	}
}

func TestParseSnapshotETS2Default(t *testing.T) {
	buf := make([]byte, MMFSize)
	buf[offSDKActive] = 1
	// offGameID fica 0 (nem 1 nem 2) -> deve cair no default ETS2
	snap, ok := parseSnapshot(buf)
	if !ok {
		t.Fatal("parseSnapshot() ok = false, want true")
	}
	if snap.Game != "Euro Truck Simulator 2" {
		t.Fatalf("Game = %q, want Euro Truck Simulator 2", snap.Game)
	}
}

func TestParseSnapshotSDKInativo(t *testing.T) {
	buf := make([]byte, MMFSize) // sdkActive = 0
	_, ok := parseSnapshot(buf)
	if ok {
		t.Fatal("parseSnapshot() ok = true com sdkActive = 0, want false")
	}
}

func TestParseSnapshotBufferCurto(t *testing.T) {
	_, ok := parseSnapshot(make([]byte, 10))
	if ok {
		t.Fatal("parseSnapshot() ok = true com buffer curto, want false")
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
}
