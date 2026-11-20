package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leonardolermen/parallax/internal/julgamento"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

func TestCheckHistoryRoundTrip(t *testing.T) {
	const corpo = "resposta completa"
	base := servidorCheck(t, http.StatusOK, corpo)
	casos := []struct {
		nome        string
		status      int
		extras      []string
		codigo      int
		resultado   veredito.Resultado
		total       int
		identicas   int
		diferentes  int
		incompletas int
	}{
		{nome: "aprovado", status: http.StatusOK, codigo: 0, resultado: "aprovado", total: 1, identicas: 1, diferentes: 0, incompletas: 0},
		{nome: "regressao", status: http.StatusInternalServerError, codigo: 1, resultado: "regressao", total: 1, identicas: 0, diferentes: 1, incompletas: 0},
		{nome: "revisar", status: http.StatusOK, extras: []string{"--max-body", "4"}, codigo: 2, resultado: "revisar", total: 1, identicas: 0, diferentes: 1, incompletas: 1},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			candidata := servidorCheck(t, caso.status, corpo)
			captura := capturaCheck(t)
			absCaptura, err := filepath.Abs(captura)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			runs := filepath.Join(dir, "runs")
			report := filepath.Join(dir, "report.md")
			args := []string{"check", "--capture", captura, "--base", base, "--candidate", candidata,
				"--runs-dir", runs, "--json", "--report", report,
				"--timeout", "5s", "--latency-pct", "10000", "--latency-min-ms", "10000", "--latency-max-ms", "10000"}
			antes := time.Now().UTC()
			codigo, stdout, stderr := rodar(append(args, caso.extras...)...)
			depois := time.Now().UTC()
			if codigo != caso.codigo {
				t.Fatalf("check: código %d, esperado %d; stderr %q", codigo, caso.codigo, stderr)
			}
			var v veredito.Veredito
			if err := json.Unmarshal([]byte(stdout), &v); err != nil {
				t.Fatalf("check --json: %v", err)
			}
			if err := v.Validar(); err != nil {
				t.Fatalf("veredito inválido: %v", err)
			}
			if v.Resultado != caso.resultado || v.ID == "" || strings.TrimSpace(v.Resumo) == "" {
				t.Fatalf("veredito: resultado %q, esperado %q; id %q, resumo %q", v.Resultado, caso.resultado, v.ID, v.Resumo)
			}
			if est := v.Estatisticas; est.Total != caso.total || est.Identicas != caso.identicas || est.Diferentes != caso.diferentes {
				t.Errorf("veredito: estatísticas %+v; esperados total %d, idênticas %d, diferentes %d",
					est, caso.total, caso.identicas, caso.diferentes)
			}

			lerArquivo := func(caminho string) []byte {
				t.Helper()
				dados, err := os.ReadFile(caminho)
				if err != nil {
					t.Fatal(err)
				}
				return dados
			}
			pastaExecucao := filepath.Join(runs, v.ID)
			var e julgamento.Execucao
			if err := json.Unmarshal(lerArquivo(filepath.Join(pastaExecucao, "execucao.json")), &e); err != nil {
				t.Fatalf("execucao.json: %v", err)
			}
			if e.ID != v.ID || e.Resultado != v.Resultado || e.Resumo != v.Resumo {
				t.Errorf("execucao.json: id, resultado ou resumo divergem do veredito: %q %q %q", e.ID, e.Resultado, e.Resumo)
			}
			if e.Data.IsZero() || e.Data.Location() != time.UTC || e.Data.Before(antes) || e.Data.After(depois) {
				t.Errorf("execucao.json: data %v, esperada em UTC entre %v e %v", e.Data, antes, depois)
			}
			if !filepath.IsAbs(e.Captura) || e.Captura != absCaptura || e.Base != base || e.Candidata != candidata {
				t.Errorf("execucao.json: captura/base/candidata = %q, %q, %q; esperado %q, %q, %q",
					e.Captura, e.Base, e.Candidata, absCaptura, base, candidata)
			}
			est := e.Estatisticas
			if est.Total != v.Estatisticas.Total || est.Identicas != v.Estatisticas.Identicas ||
				est.Diferentes != v.Estatisticas.Diferentes || est.Latencia != v.Estatisticas.Latencia {
				t.Errorf("execucao.json: estatísticas comuns %+v divergem do veredito %+v", est, v.Estatisticas)
			}
			if est.Incompletas != caso.incompletas {
				t.Errorf("execucao.json: incompletas %d, esperado %d", est.Incompletas, caso.incompletas)
			}

			if gravado := lerArquivo(filepath.Join(pastaExecucao, "veredito.json")); string(gravado) != stdout {
				t.Errorf("veredito.json difere do stdout de check --json")
			}
			codigo, jsonHistorico, stderr := rodar("history", "show", v.ID, "--json", "--runs-dir", runs)
			if codigo != 0 || jsonHistorico != stdout {
				t.Errorf("history show --json: código %d, JSON igual ao check: %t; stderr %q", codigo, jsonHistorico == stdout, stderr)
			}

			laudo := string(lerArquivo(report))
			if !strings.HasPrefix(laudo, "# Laudo Parallax "+v.ID+"\n") {
				t.Errorf("report sem cabeçalho da execução %q", v.ID)
			}
			if gravado := lerArquivo(filepath.Join(pastaExecucao, "laudo.md")); string(gravado) != laudo {
				t.Errorf("laudo.md difere do report")
			}
			codigo, laudoHistorico, stderr := rodar("history", "show", v.ID, "--runs-dir", runs)
			if codigo != 0 || laudoHistorico != laudo {
				t.Errorf("history show: código %d, laudo igual ao report: %t; stderr %q", codigo, laudoHistorico == laudo, stderr)
			}

			codigo, lista, stderr := rodar("history", "--runs-dir", runs)
			linhas := strings.Split(strings.TrimSpace(lista), "\n")
			if codigo != 0 || len(linhas) != 2 {
				t.Fatalf("history: código %d, listagem %q; stderr %q", codigo, lista, stderr)
			}
			colunas := strings.Fields(linhas[1])
			if len(colunas) < 3 || colunas[0] != v.ID || colunas[2] != string(caso.resultado) ||
				!strings.HasSuffix(linhas[1], v.Resumo) {
				t.Errorf("history: registro %q sem id %q, resultado %q ou resumo %q", linhas[1], v.ID, caso.resultado, v.Resumo)
			}
		})
	}
}
