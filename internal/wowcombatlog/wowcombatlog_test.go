package wowcombatlog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLineZoneChange(t *testing.T) {
	p := &Poller{}
	p.parseLine(`4/9 05:05:07.807  ZONE_CHANGE,37,"Elwynn Forest",1`)
	snap := p.Current()
	if snap.Zone != "Elwynn Forest" {
		t.Fatalf("Zone = %q, want Elwynn Forest", snap.Zone)
	}
}

func TestParseLineEncounterStartEnd(t *testing.T) {
	p := &Poller{}
	p.parseLine(`4/9 05:05:07.807  ENCOUNTER_START,1146,"Randolph Moloch",1,5,34`)
	snap := p.Current()
	if !snap.InEncounter {
		t.Fatal("InEncounter = false, want true after ENCOUNTER_START")
	}
	if snap.Encounter != "Randolph Moloch" {
		t.Fatalf("Encounter = %q, want Randolph Moloch", snap.Encounter)
	}
	if snap.Difficulty != "Normal" {
		t.Fatalf("Difficulty = %q, want Normal", snap.Difficulty)
	}
	if snap.GroupSize != 5 {
		t.Fatalf("GroupSize = %d, want 5", snap.GroupSize)
	}
	if !snap.Ativa() {
		t.Fatal("Ativa() = false logo após ENCOUNTER_START")
	}

	p.parseLine(`4/9 05:15:07.807  ENCOUNTER_END,1146,"Randolph Moloch",1,5,1,600000`)
	snap = p.Current()
	if snap.InEncounter {
		t.Fatal("InEncounter = true, want false after ENCOUNTER_END")
	}
	if snap.Ativa() {
		t.Fatal("Ativa() = true depois do ENCOUNTER_END, want false")
	}
}

func TestParseLineIgnoraLixo(t *testing.T) {
	p := &Poller{}
	p.parseLine("linha sem separador de duplo espaco")
	p.parseLine("")
	if p.Current().Ativa() {
		t.Fatal("linhas invalidas nao deviam gerar estado ativo")
	}
}

func TestSnapshotAtivaExpira(t *testing.T) {
	var s Snapshot
	if s.Ativa() {
		t.Fatal("Snapshot zero nao devia estar ativo")
	}
	s.InEncounter = true
	s.UpdatedAt = time.Now().Add(-(Validade + time.Second))
	if s.Ativa() {
		t.Fatal("Snapshot velho nao devia estar ativo")
	}
	s.UpdatedAt = time.Now()
	if !s.Ativa() {
		t.Fatal("Snapshot recente devia estar ativo")
	}
}

// TestPollerTailFile simula o jogo escrevendo linhas novas aos poucos num
// arquivo real, verificando que o Poller acompanha (tail -f) corretamente,
// inclusive quando o arquivo é truncado/recriado (novo login).
func TestPollerTailFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WoWCombatLog.txt")
	if err := os.WriteFile(path, []byte(`4/9 05:00:00.000  ZONE_CHANGE,37,"Elwynn Forest",1`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NovoPoller(path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx, 20*time.Millisecond)

	waitFor(t, func() bool { return p.Current().Zone == "Elwynn Forest" })

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`4/9 05:05:00.000  ENCOUNTER_START,1146,"Randolph Moloch",1,5,34` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	waitFor(t, func() bool { return p.Current().Ativa() })

	// simula reinício do jogo (arquivo recriado do zero, menor que antes).
	if err := os.WriteFile(path, []byte(`4/9 06:00:00.000  ZONE_CHANGE,84,"Stormwind City",0`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return p.Current().Zone == "Stormwind City" })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condição não atingida a tempo")
}
