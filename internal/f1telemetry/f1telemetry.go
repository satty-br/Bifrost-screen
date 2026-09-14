// Package f1telemetry recebe a telemetria ao vivo dos jogos F1 (Codemasters/EA
// Sports F1 22 em diante) pelo protocolo oficial de "UDP Telemetry": o
// próprio jogo manda, sozinho, pacotes binários por UDP para o endereço/porta
// configurados em Configurações > Telemetria dentro do jogo. Não precisa de
// injeção, leitura de memória nem burlar nada — é o mesmo mecanismo usado por
// simuladores de cockpit e overlays de telemetria oficiais.
//
// Formato dos pacotes documentado oficialmente pela Codemasters/EA (link
// "F1 24 UDP Specification" nos fóruns oficiais do jogo); layout de bytes
// conferido também com implementações de referência de código aberto.
//
// Diferente do GSI (CS2/Dota2), aqui o Bifrost só escuta: quem aponta o jogo
// pra cá é o próprio jogador, configurando IP 127.0.0.1 e a porta em uso
// (DefaultPort, que já é a porta padrão sugerida pelo jogo).
package f1telemetry

import (
	"encoding/binary"
	"math"
	"net"
	"sync"
	"time"
)

// DefaultPort é a porta padrão usada pelos jogos F1 pra mandar telemetria —
// mesma sugestão de porta que já vem pré-marcada no menu do jogo.
const DefaultPort = 20777

// Validade: sem pacote novo por esse tempo (jogo fechou, saiu da sessão,
// pausou no menu principal...), a leitura é considerada velha.
const Validade = 5 * time.Second

const (
	headerSize       = 29
	carTelemetrySize = 60
	lapDataSize      = 57
	maxCars          = 22
)

// IDs de pacote conforme a especificação oficial (só os que a gente lê).
const (
	packetIDSession      = 1
	packetIDLapData      = 2
	packetIDCarTelemetry = 6
)

// Snapshot é o resumo da sessão/carro do jogador, mostrado no painel/tela.
type Snapshot struct {
	UpdatedAt time.Time

	Track       string
	SessionType string
	TotalLaps   int

	Speed     int // km/h
	Gear      int // -1 = ré, 0 = neutro, 1-8
	EngineRPM int
	Throttle  float64 // 0.0 a 1.0
	Brake     float64 // 0.0 a 1.0
	DRS       bool

	CarPosition      int
	CurrentLapNum    int
	CurrentLapTimeMS uint32
	LastLapTimeMS    uint32
	Sector           int // 1, 2 ou 3
}

// Ativa diz se a leitura é recente o bastante pra ser considerada "ao vivo".
func (s Snapshot) Ativa() bool {
	return !s.UpdatedAt.IsZero() && time.Since(s.UpdatedAt) < Validade
}

// Server escuta os pacotes de telemetria UDP enquanto estiver ligado.
type Server struct {
	mu   sync.RWMutex
	snap Snapshot

	conn *net.UDPConn
	port int
}

// NovoServer cria um servidor parado; chame Start para ligar.
func NovoServer() *Server { return &Server{} }

// Start liga o listener UDP em 127.0.0.1:port (0 = o sistema escolhe uma porta livre).
func (s *Server) Start(port int) error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return err
	}
	s.conn = conn
	s.port = conn.LocalAddr().(*net.UDPAddr).Port
	go s.loop()
	return nil
}

// Porta devolve a porta em que o servidor está escutando (0 se parado).
func (s *Server) Porta() int { return s.port }

// Stop desliga o listener.
func (s *Server) Stop() {
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

// Current devolve a última leitura recebida, ou Snapshot{} se estiver velha
// demais (ver Snapshot.Ativa).
func (s *Server) Current() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

func (s *Server) loop() {
	buf := make([]byte, 2048)
	for {
		n, err := s.conn.Read(buf)
		if err != nil {
			return
		}
		s.handlePacket(buf[:n])
	}
}

func (s *Server) handlePacket(body []byte) {
	if len(body) < headerSize {
		return
	}
	packetID := body[6]
	playerCarIndex := int(body[27])
	if playerCarIndex < 0 || playerCarIndex >= maxCars {
		return
	}
	switch packetID {
	case packetIDCarTelemetry:
		s.parseCarTelemetry(body, playerCarIndex)
	case packetIDLapData:
		s.parseLapData(body, playerCarIndex)
	case packetIDSession:
		s.parseSession(body)
	}
}

func (s *Server) parseCarTelemetry(body []byte, playerCarIndex int) {
	off := headerSize + playerCarIndex*carTelemetrySize
	if len(body) < off+carTelemetrySize {
		return
	}
	d := body[off : off+carTelemetrySize]
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.UpdatedAt = time.Now()
	s.snap.Speed = int(binary.LittleEndian.Uint16(d[0:2]))
	s.snap.Throttle = float64(math.Float32frombits(binary.LittleEndian.Uint32(d[2:6])))
	s.snap.Brake = float64(math.Float32frombits(binary.LittleEndian.Uint32(d[10:14])))
	s.snap.Gear = int(int8(d[15]))
	s.snap.EngineRPM = int(binary.LittleEndian.Uint16(d[16:18]))
	s.snap.DRS = d[18] != 0
}

func (s *Server) parseLapData(body []byte, playerCarIndex int) {
	off := headerSize + playerCarIndex*lapDataSize
	if len(body) < off+lapDataSize {
		return
	}
	d := body[off : off+lapDataSize]
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.UpdatedAt = time.Now()
	s.snap.LastLapTimeMS = binary.LittleEndian.Uint32(d[0:4])
	s.snap.CurrentLapTimeMS = binary.LittleEndian.Uint32(d[4:8])
	s.snap.CarPosition = int(d[32])
	s.snap.CurrentLapNum = int(d[33])
	s.snap.Sector = int(d[36]) + 1
}

func (s *Server) parseSession(body []byte) {
	const off = headerSize
	if len(body) < off+8 {
		return
	}
	totalLaps := int(body[off+3])
	sessionType := int(body[off+6])
	trackID := int(int8(body[off+7]))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.UpdatedAt = time.Now()
	s.snap.TotalLaps = totalLaps
	s.snap.SessionType = sessionTypeName(sessionType)
	s.snap.Track = trackName(trackID)
}
