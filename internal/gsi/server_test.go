package gsi

import (
	"bytes"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestServerEndToEnd(t *testing.T) {
	s := NovoServer("")
	if err := s.Start(0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	if m := s.Current(); m.Game != "" {
		t.Fatalf("servidor novo não devia ter partida, veio %+v", m)
	}

	post := func(body string) *http.Response {
		req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(s.Porta())+"/gsi", bytes.NewReader([]byte(body)))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		return resp
	}

	resp := post(cs2SamplePayload)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, queria 200", resp.StatusCode)
	}
	m := s.Current()
	if m.Game != "cs2" || m.CS2 == nil || m.CS2.Map != "de_mirage" {
		t.Fatalf("partida não ficou registrada certo: %+v", m)
	}
	if !m.Ativa() {
		t.Error("partida recém-recebida devia estar ativa")
	}

	resp2 := post(dota2SamplePayload)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, queria 200", resp2.StatusCode)
	}
	m2 := s.Current()
	if m2.Game != "dota2" || m2.Dota2 == nil || m2.Dota2.Hero != "axe" {
		t.Fatalf("troca pro Dota2 não ficou certa: %+v", m2)
	}
}

func TestServerTokenErrado(t *testing.T) {
	s := NovoServer("")
	if err := s.Start(0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	comTokenErrado := `{"provider":{"appid":730},"auth":{"token":"errado"},"map":{"name":"de_mirage"}}`
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(s.Porta())+"/gsi", bytes.NewReader([]byte(comTokenErrado)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, queria 403 (token errado)", resp.StatusCode)
	}
	if s.Current().Game != "" {
		t.Error("payload com token errado não devia atualizar a partida")
	}
}

func TestMatchAtivaExpira(t *testing.T) {
	m := Match{Game: "cs2", UpdatedAt: time.Now().Add(-time.Hour), CS2: &CS2State{}}
	if m.Ativa() {
		t.Error("partida velha não devia contar como ativa")
	}
}
