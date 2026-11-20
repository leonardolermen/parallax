package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leonardolermen/parallax/internal/julgamento"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

var errStdoutCLI = errors.New("falha sentinela de stdout")

type stdoutComErro struct{}

func (stdoutComErro) Write([]byte) (int, error) {
	return 0, errStdoutCLI
}

func argsCheckErros(captura, alvo, runs string) []string {
	return []string{
		"--capture", captura, "--base", alvo, "--candidate", alvo, "--runs-dir", runs,
		"--timeout", "10s", "--latency-min-ms", "10000", "--latency-max-ms", "0",
	}
}

func lerLaudoPreservado(t *testing.T, runs string) []byte {
	t.Helper()
	ids := execucoesGravadas(t, runs)
	if len(ids) != 1 {
		t.Fatalf("esperava uma execução preservada, obtive %v", ids)
	}
	e, v, laudo, err := (julgamento.Historico{Dir: runs}).Ler(ids[0])
	if err != nil {
		t.Fatalf("lendo execução preservada: %v", err)
	}
	if err := v.Validar(); err != nil {
		t.Fatalf("veredito preservado inválido: %v", err)
	}
	if e.Resultado != veredito.Aprovado || v.Resultado != veredito.Aprovado ||
		!strings.Contains(string(laudo), "# Laudo Parallax "+ids[0]) ||
		!strings.Contains(string(laudo), "**APROVADO**") {
		t.Fatalf("execução %s, veredito %s, laudo %q", e.Resultado, v.Resultado, laudo)
	}
	return laudo
}

// Ignorar o erro de stdout ou sair antes de gravar o histórico e o report perde o resultado.
func TestCheckErroSaida(t *testing.T) {
	alvo := servidorCheck(t, http.StatusOK, "ok")
	casos := []struct {
		nome  string
		flags []string
	}{
		{nome: "Markdown"},
		{nome: "JSON", flags: []string{"--json"}},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			dir := t.TempDir()
			runs := filepath.Join(dir, "runs")
			report := filepath.Join(dir, "laudo.md")
			args := append(argsCheckErros(capturaCheck(t), alvo, runs), "--report", report)
			args = append(args, caso.flags...)
			var stderr bytes.Buffer
			codigo := checar(context.Background(), args, stdoutComErro{}, &stderr)
			if codigo != 4 || !strings.Contains(stderr.String(), errStdoutCLI.Error()) {
				t.Fatalf("código %d, stderr %q", codigo, stderr.String())
			}
			laudo := lerLaudoPreservado(t, runs)
			dados, err := os.ReadFile(report)
			if err != nil || !bytes.Equal(dados, laudo) {
				t.Fatalf("report difere do laudo preservado: %v, conteúdo %q", err, dados)
			}
		})
	}
}

// Falhar ao gravar o report deve afetar o código de saída e preservar o resultado.
func TestCheckErroReport(t *testing.T) {
	alvo := servidorCheck(t, http.StatusOK, "ok")
	runs := filepath.Join(t.TempDir(), "runs")
	report := t.TempDir()
	args := append(argsCheckErros(capturaCheck(t), alvo, runs), "--report", report)
	codigo, stdout, stderr := checarTeste(args...)
	if codigo != 4 || !strings.Contains(stderr, report) {
		t.Fatalf("código %d, stderr %q", codigo, stderr)
	}
	laudo := lerLaudoPreservado(t, runs)
	if stdout != string(laudo) {
		t.Fatalf("stdout difere do laudo preservado: %q", stdout)
	}
}

// Iniciar replay antes de validar toda a captura pode enviar uma troca válida antes do erro.
func TestCheckEntradaInvalidaSemTrafego(t *testing.T) {
	casos := []struct {
		nome, dados, diagnostico string
	}{
		{nome: "vazia", diagnostico: "captura vazia"},
		{
			nome:        "linha inválida após linha válida",
			dados:       `{"metodo":"GET","caminho":"/pedidos","status":200}` + "\n{\n",
			diagnostico: "linha 2",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			var envios atomic.Int32
			servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				envios.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(servidor.Close)
			dir := t.TempDir()
			captura := filepath.Join(dir, "captura.jsonl")
			if err := os.WriteFile(captura, []byte(caso.dados), 0o600); err != nil {
				t.Fatal(err)
			}
			runs := filepath.Join(dir, "runs")
			codigo, stdout, stderr := checarTeste(argsCheckErros(captura, servidor.URL, runs)...)
			if codigo != 4 || stdout != "" || !strings.Contains(stderr, caso.diagnostico) {
				t.Errorf("código %d, stdout %q, stderr %q", codigo, stdout, stderr)
			}
			if total := envios.Load(); total != 0 {
				t.Errorf("captura inválida disparou %d requisições", total)
			}
			if ids := execucoesGravadas(t, runs); len(ids) != 0 {
				t.Errorf("gravou histórico com captura inválida: %v", ids)
			}
		})
	}
}

// Desvincular o request do contexto aguarda o timeout HTTP ou produz um resultado indevido.
func TestCheckCancelamento(t *testing.T) {
	casos := []struct {
		nome         string
		antesDoCheck bool
	}{
		{nome: "contexto já cancelado", antesDoCheck: true},
		{nome: "durante requisição"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			dir := t.TempDir()
			captura := capturaCheck(t)
			recebida := make(chan struct{}, 1)
			liberar := make(chan struct{})
			servidor := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				select {
				case recebida <- struct{}{}:
				default:
				}
				select {
				case <-r.Context().Done():
				case <-liberar:
				}
			}))
			t.Cleanup(servidor.Close)
			runs := filepath.Join(dir, "runs")
			report := filepath.Join(dir, "laudo.md")
			args := append(argsCheckErros(captura, servidor.URL, runs), "--report", report)
			ctx, cancelar := context.WithCancel(context.Background())
			defer cancelar()
			if caso.antesDoCheck {
				cancelar()
			}
			var stdout, stderr bytes.Buffer
			concluido := make(chan int, 1)
			prazo := time.NewTimer(5 * time.Second)
			defer prazo.Stop()
			terminou := make(chan struct{})
			t.Cleanup(func() {
				cancelar()
				close(liberar)
				select {
				case <-terminou:
				case <-time.After(5 * time.Second):
					t.Error("check não terminou durante limpeza")
				}
			})
			go func() {
				defer close(terminou)
				concluido <- checar(ctx, args, &stdout, &stderr)
			}()
			if !caso.antesDoCheck {
				select {
				case <-recebida:
					cancelar()
				case codigo := <-concluido:
					t.Fatalf("check terminou antes da requisição: código %d, stderr %q", codigo, stderr.String())
				case <-prazo.C:
					t.Fatal("servidor não recebeu requisição em 5s")
				}
			}
			select {
			case codigo := <-concluido:
				if codigo != 4 || stdout.Len() != 0 || !strings.Contains(stderr.String(), context.Canceled.Error()) {
					t.Errorf("código %d, stdout %q, stderr %q", codigo, stdout.String(), stderr.String())
				}
			case <-prazo.C:
				t.Fatal("check não terminou em 5s")
			}
			if caso.antesDoCheck && len(recebida) != 0 {
				t.Error("check enviou requisição com o contexto já cancelado")
			}
			if ids := execucoesGravadas(t, runs); len(ids) != 0 {
				t.Errorf("gravou histórico após cancelamento: %v", ids)
			}
			if _, err := os.Stat(report); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("report deveria estar ausente após cancelamento: %v", err)
			}
		})
	}
}
