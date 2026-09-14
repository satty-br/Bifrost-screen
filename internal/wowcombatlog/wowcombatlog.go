// Package wowcombatlog acompanha o "Combat Log" do World of Warcraft — um
// arquivo de texto (WoWCombatLog.txt) que o próprio jogo escreve sozinho
// enquanto o registro de combate estiver ligado (comando /combatlog, ou
// automaticamente por addons como o Details!). É o mesmo arquivo que
// ferramentas como Details!, Recount e o Warcraft Logs usam — o Bifrost só
// lê o arquivo, sem addon, sem injeção e sem ler a memória do jogo.
//
// A partir do patch 12.0 ("Midnight"), a Blizzard tirou dos addons o acesso
// ao evento COMBAT_LOG_EVENT em tempo real, mas a escrita do arquivo em disco
// não foi afetada — é um mecanismo separado, ligado pelo comando /combatlog
// (ou LoggingCombat(true)), independente da API de addons.
//
// Documentação oficial dos campos: https://warcraft.wiki.gg/wiki/Event:COMBAT_LOG_EVENT
package wowcombatlog

import (
	"context"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Validade: sem ENCOUNTER_START/END novo por esse tempo, a luta é
// considerada encerrada (o jogador saiu, desconectou, ou o log parou).
const Validade = 30 * time.Second

// Snapshot é o resumo da luta atual, mostrado no painel/tela. Só faz sentido
// mostrar a tela ao vivo durante uma luta de encontro (boss) — fora disso o
// jogo é mundo aberto, sem um "estado de partida" equivalente ao de um MOBA/FPS.
type Snapshot struct {
	UpdatedAt time.Time

	Zone        string // última zona conhecida (ZONE_CHANGE), mesmo fora de luta
	InEncounter bool
	Encounter   string
	Difficulty  string
	GroupSize   int
	StartedAt   time.Time
}

// Ativa diz se há uma luta de encontro em andamento (recente).
func (s Snapshot) Ativa() bool {
	return s.InEncounter && !s.UpdatedAt.IsZero() && time.Since(s.UpdatedAt) < Validade
}

// Poller acompanha o arquivo de log, lendo só o que foi escrito de novo
// desde a última leitura (como um "tail -f").
type Poller struct {
	customPath string // vazio = tenta detectar sozinho nos caminhos padrão

	mu   sync.RWMutex
	snap Snapshot
}

// NovoPoller cria um poller parado; chame Start para ligar. customPath pode
// apontar direto pro WoWCombatLog.txt se a instalação não estiver num
// caminho padrão (vazio = detecção automática).
func NovoPoller(customPath string) *Poller {
	return &Poller{customPath: customPath}
}

// Current devolve a última leitura, ou Snapshot{} se estiver velha demais.
func (p *Poller) Current() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.snap
}

// Start liga a leitura periódica do arquivo; para sozinho quando ctx é
// cancelado. Se o arquivo não for encontrado, simplesmente não há dados —
// não é um erro fatal pro resto do app (o jogo pode nem estar instalado).
func (p *Poller) Start(ctx context.Context, interval time.Duration) {
	go p.loop(ctx, interval)
}

func (p *Poller) loop(ctx context.Context, interval time.Duration) {
	var (
		path     string
		offset   int64
		leftover []byte
	)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		current := p.customPath
		if current == "" {
			current = findLogFile()
		}
		if current == "" {
			continue
		}
		if current != path {
			path = current
			offset = 0
			leftover = nil
		}

		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Size() < offset {
			// jogo reiniciou/relogou: o arquivo foi recriado do zero.
			offset = 0
			leftover = nil
		}
		if info.Size() == offset {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			continue
		}
		buf := make([]byte, info.Size()-offset)
		n, _ := io.ReadFull(f, buf)
		f.Close()
		offset += int64(n)

		data := append(leftover, buf[:n]...)
		lines := strings.Split(string(data), "\n")
		if !strings.HasSuffix(string(data), "\n") {
			leftover = []byte(lines[len(lines)-1])
			lines = lines[:len(lines)-1]
		} else {
			leftover = nil
		}
		for _, line := range lines {
			p.parseLine(strings.TrimRight(line, "\r"))
		}
	}
}

func (p *Poller) parseLine(line string) {
	// Formato: "<data hora.ms>  <evento>,<campo1>,<campo2>,..." — dois
	// espaços separam o timestamp do resto (documentado com exemplos em
	// warcraft.wiki.gg/wiki/Event:COMBAT_LOG_EVENT#Advanced_Combat_Log).
	idx := strings.Index(line, "  ")
	if idx < 0 {
		return
	}
	rest := line[idx+2:]
	r := csv.NewReader(strings.NewReader(rest))
	r.FieldsPerRecord = -1
	fields, err := r.Read()
	if err != nil || len(fields) == 0 {
		return
	}

	now := time.Now()
	switch fields[0] {
	case "ZONE_CHANGE":
		if len(fields) < 3 {
			return
		}
		p.mu.Lock()
		p.snap.Zone = fields[2]
		p.mu.Unlock()
	case "ENCOUNTER_START":
		if len(fields) < 5 {
			return
		}
		groupSize, _ := strconv.Atoi(fields[4])
		p.mu.Lock()
		p.snap.InEncounter = true
		p.snap.Encounter = fields[2]
		p.snap.Difficulty = difficultyName(fields[3])
		p.snap.GroupSize = groupSize
		p.snap.StartedAt = now
		p.snap.UpdatedAt = now
		p.mu.Unlock()
	case "ENCOUNTER_END":
		p.mu.Lock()
		p.snap.InEncounter = false
		p.snap.UpdatedAt = now
		p.mu.Unlock()
	}
}

// candidatePaths são os locais padrão de instalação do Battle.net — o jogo
// não tem uma API de descoberta como a Steam, então tentamos os caminhos
// mais comuns (Windows e macOS) para cada variante (retail/classic/era).
func candidatePaths() []string {
	var roots []string
	switch runtime.GOOS {
	case "windows":
		roots = []string{
			`C:\Program Files (x86)\World of Warcraft`,
			`C:\Program Files\World of Warcraft`,
		}
	case "darwin":
		roots = []string{"/Applications/World of Warcraft"}
	default:
		return nil
	}
	variants := []string{"_retail_", "_classic_", "_classic_era_"}
	var paths []string
	for _, root := range roots {
		for _, v := range variants {
			paths = append(paths, filepath.Join(root, v, "Logs", "WoWCombatLog.txt"))
		}
	}
	return paths
}

func findLogFile() string {
	for _, p := range candidatePaths() {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}
