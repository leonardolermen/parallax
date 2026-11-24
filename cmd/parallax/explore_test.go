package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leonardolermen/parallax/pkg/veredito"
)

// servicoExplore devolve uma lista e o detalhe de cada item; versao muda o
// detalhe do item 2 (a "regressão" da candidata).
func servicoExplore(t *testing.T, quebrado bool) string {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1},{"id":2}]`)
	})
	mux.HandleFunc("GET /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		if quebrado && r.PathValue("id") == "2" {
			http.Error(w, "falhou", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%s}`, r.PathValue("id"))
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s.URL
}

func explorarTeste(args ...string) (int, string) {
	var stderr bytes.Buffer
	return explorar(context.Background(), args, &stderr), stderr.String()
}

func TestExploreDepoisCheck(t *testing.T) {
	fonte := t.TempDir()
	if err := os.WriteFile(filepath.Join(fonte, "main.go"),
		[]byte("mux.HandleFunc(\"GET /api/items\", l)\nmux.HandleFunc(\"GET /api/items/{id}\", d)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := servicoExplore(t, false)
	cap := filepath.Join(t.TempDir(), "captura.jsonl")
	codigo, saida := explorarTeste("--target", base, "--source", fonte, "--out", cap)
	if codigo != 0 {
		t.Fatalf("explore saiu com %d: %s", codigo, saida)
	}
	if !strings.Contains(saida, "3 trocas gravadas") {
		t.Fatalf("saída: %s", saida)
	}

	runs := t.TempDir()
	if c, _, e := checarTeste("--capture", cap, "--base", base, "--candidate", servicoExplore(t, false), "--runs-dir", runs); c != veredito.SaidaAprovado {
		t.Fatalf("check igual saiu com %d: %s", c, e)
	}
	if c, _, e := checarTeste("--capture", cap, "--base", base, "--candidate", servicoExplore(t, true), "--runs-dir", runs); c != veredito.SaidaRegressao {
		t.Fatalf("check com regressão saiu com %d: %s", c, e)
	}
}

func TestExploreErrosDeUso(t *testing.T) {
	casos := [][]string{
		{"--out", "x.jsonl"},
		{"--target", "http://localhost:1"},
		{"--target", "http://localhost:1", "--out", "x", "--source", filepath.Join(t.TempDir(), "nao-existe")},
		{"--target", "http://localhost:1", "--out", "x", "--exclude", "("},
	}
	for _, args := range casos {
		if c, _ := explorarTeste(args...); c != veredito.SaidaErro {
			t.Errorf("%v saiu com %d", args, c)
		}
	}
}

func TestExploreSemNadaRespondendo(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	cap := filepath.Join(t.TempDir(), "captura.jsonl")
	codigo, saida := explorarTeste("--target", s.URL, "--out", cap)
	if codigo != veredito.SaidaErro || !strings.Contains(saida, "nenhuma rota") {
		t.Fatalf("saiu com %d: %s", codigo, saida)
	}
	if _, err := os.Stat(cap); err == nil {
		t.Error("criou a captura vazia")
	}
}
