package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReplayCLI(t *testing.T) {
	var chamadas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas.Add(1)
		if r.Header.Get("Authorization") != "Bearer novo" || r.Header.Get("X-Test") != "sim" {
			t.Error("cabecalhos injetados ausentes")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	capture := filepath.Join(t.TempDir(), "captura.jsonl")
	if err := os.WriteFile(capture, []byte("{\"metodo\":\"GET\",\"caminho\":\"/status\",\"status\":200}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--capture", capture, "--base", srv.URL, "--candidate", srv.URL,
		"--header", "Authorization: Bearer novo", "--header", "X-Test: sim"}
	for _, arquivo := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		atual := append([]string{}, args...)
		out := filepath.Join(t.TempDir(), "replay.json")
		if arquivo {
			atual = append(atual, "--out", out)
		}
		if codigo := repetir(context.Background(), atual, &stdout, &stderr); codigo != 0 {
			t.Fatalf("codigo %d: %s", codigo, &stderr)
		}
		dados := stdout.Bytes()
		if arquivo {
			var err error
			dados, err = os.ReadFile(out)
			if err != nil || stdout.Len() != 0 {
				t.Fatalf("arquivo: %v, stdout=%q", err, &stdout)
			}
		}
		var resultado struct {
			ID           string                                  `json:"id"`
			Achados      []any                                   `json:"achados"`
			Estatisticas struct{ Total, Identicas, Puladas int } `json:"estatisticas"`
		}
		if err := json.Unmarshal(dados, &resultado); err != nil {
			t.Fatal(err)
		}
		if len(resultado.ID) != 26 || resultado.Achados == nil || resultado.Estatisticas.Total != 1 || resultado.Estatisticas.Identicas != 1 {
			t.Fatalf("resultado: %s", dados)
		}
	}
	if chamadas.Load() != 4 {
		t.Fatalf("chamadas=%d", chamadas.Load())
	}
}

func TestReplayCLIErrosSemTrafego(t *testing.T) {
	var chamadas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { chamadas.Add(1) }))
	defer srv.Close()
	capture := filepath.Join(t.TempDir(), "captura.jsonl")
	if err := os.WriteFile(capture, []byte("{\"metodo\":\"GET\",\"caminho\":\"/ok\"}\nlinha invalida\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--capture", capture, "--base", srv.URL, "--candidate", srv.URL}
	casos := [][]string{
		{}, {"--unknown"}, {"--capture", capture},
		append(append([]string{}, base...), "--workers", "0"),
		append(append([]string{}, base...), "--timeout", "0"),
		append(append([]string{}, base...), "--max-body", "0"),
		append(append([]string{}, base...), "--latency-pct", "NaN"),
		append(append([]string{}, base...), "--header", "invalido"),
		append(append([]string{}, base...), "--header", "X-Test: a\r\nb"),
		append(append([]string{}, base...), "--header", "X-Test: a\r\n"),
		append(append([]string{}, base...), "sobra"), base,
	}
	for _, args := range casos {
		var stdout, stderr bytes.Buffer
		if codigo := repetir(context.Background(), args, &stdout, &stderr); codigo != 4 || stderr.Len() == 0 {
			t.Errorf("%v: codigo=%d stderr=%s", args, codigo, &stderr)
		}
		if stdout.Len() != 0 {
			t.Errorf("erro produziu resultado: %s", &stdout)
		}
	}
	if chamadas.Load() != 0 {
		t.Fatalf("captura invalida enviou %d", chamadas.Load())
	}
	var stdout, stderr bytes.Buffer
	if codigo := repetir(context.Background(), []string{"--help"}, &stdout, &stderr); codigo != 0 || !strings.Contains(stderr.String(), "capture") {
		t.Fatalf("ajuda: %d %s", codigo, &stderr)
	}
}

func TestReplayCLIComAchadosSaiZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	capture := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(capture, []byte(`{"metodo":"GET","caminho":"/"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if codigo := repetir(context.Background(), []string{"--capture", capture, "--base", srv.URL, "--candidate", srv.URL}, &stdout, &stderr); codigo != 0 || !strings.Contains(stdout.String(), "erro_novo") {
		t.Fatalf("codigo=%d stdout=%s stderr=%s", codigo, &stdout, &stderr)
	}
}

type escritorReplayFalho struct{}

func (escritorReplayFalho) Write(p []byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestReplayCLIErrosDeSaida(t *testing.T) {
	var chamadas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas.Add(1)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	capture := filepath.Join(t.TempDir(), "c.jsonl")
	conteudo := []byte(`{"metodo":"GET","caminho":"/"}`)
	if err := os.WriteFile(capture, conteudo, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--capture", capture, "--base", srv.URL, "--candidate", srv.URL}
	var stdout, stderr bytes.Buffer
	if codigo := repetir(context.Background(), append(append([]string{}, args...), "--out", capture), &stdout, &stderr); codigo != 4 {
		t.Fatalf("sobrescrever captura: %d", codigo)
	}
	depois, err := os.ReadFile(capture)
	if err != nil || !bytes.Equal(depois, conteudo) || chamadas.Load() != 0 {
		t.Fatalf("captura protegida: %s %v chamadas=%d", depois, err, chamadas.Load())
	}
	stderr.Reset()
	if codigo := repetir(context.Background(), args, escritorReplayFalho{}, &stderr); codigo != 4 || !strings.Contains(stderr.String(), "closed pipe") {
		t.Fatalf("erro escrita: %d %s", codigo, &stderr)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	antes := chamadas.Load()
	stderr.Reset()
	if codigo := repetir(ctx, args, &stdout, &stderr); codigo != 4 || chamadas.Load() != antes {
		t.Fatalf("cancelado: codigo=%d chamadas=%d, antes=%d", codigo, chamadas.Load(), antes)
	}
}
