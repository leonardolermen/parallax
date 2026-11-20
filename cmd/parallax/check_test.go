package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leonardolermen/parallax/pkg/veredito"
)

// servidorCheck responde 200 com o corpo dado em qualquer rota.
func servidorCheck(t *testing.T, status int, corpo string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(corpo))
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func capturaCheck(t *testing.T, linhas ...string) string {
	t.Helper()
	if len(linhas) == 0 {
		linhas = []string{`{"metodo":"GET","caminho":"/pedidos","status":200}`}
	}
	c := filepath.Join(t.TempDir(), "captura.jsonl")
	if err := os.WriteFile(c, []byte(strings.Join(linhas, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return c
}

func checarTeste(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	codigo := checar(context.Background(), args, &stdout, &stderr)
	return codigo, stdout.String(), stderr.String()
}

func execucoesGravadas(t *testing.T, runs string) []string {
	t.Helper()
	ds, err := os.ReadDir(runs)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, d := range ds {
		ids = append(ids, d.Name())
	}
	return ids
}

func TestCheckResultados(t *testing.T) {
	base := servidorCheck(t, 200, `{"ok":true}`)
	casos := []struct {
		nome      string
		candidata string
		extra     []string
		codigo    int
		laudo     string
	}{
		{"aprovado", base, nil, 0, "**APROVADO**"},
		{"regressao", servidorCheck(t, 500, "erro"), nil, 1, "**REGRESSAO**"},
		{"revisar", servidorCheck(t, 200, `{"ok":false}`), nil, 2, "**REVISAR**"},
		{"incompleta", base, []string{"--max-body", "4"}, 2, "Incompletas | 1"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			runs := filepath.Join(t.TempDir(), "runs")
			args := append([]string{"--capture", capturaCheck(t), "--base", base, "--candidate", c.candidata, "--runs-dir", runs}, c.extra...)
			codigo, stdout, stderr := checarTeste(args...)
			if codigo != c.codigo {
				t.Fatalf("código %d, esperado %d; stderr %q", codigo, c.codigo, stderr)
			}
			if !strings.Contains(stdout, "# Laudo Parallax") || !strings.Contains(stdout, c.laudo) {
				t.Fatalf("laudo sem %q:\n%s", c.laudo, stdout)
			}
			ids := execucoesGravadas(t, runs)
			if len(ids) != 1 || !strings.Contains(stderr, ids[0]) {
				t.Fatalf("histórico %v, stderr %q", ids, stderr)
			}
			for _, nome := range []string{"veredito.json", "laudo.md", "execucao.json"} {
				if _, err := os.Stat(filepath.Join(runs, ids[0], nome)); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func TestCheckJSONEReport(t *testing.T) {
	base := servidorCheck(t, 200, "ok")
	runs := filepath.Join(t.TempDir(), "runs")
	report := filepath.Join(t.TempDir(), "laudo.md")
	codigo, stdout, stderr := checarTeste("--capture", capturaCheck(t), "--base", base, "--candidate", base,
		"--runs-dir", runs, "--json", "--report", report)
	if codigo != 0 {
		t.Fatalf("código %d: %s", codigo, stderr)
	}
	var v veredito.Veredito
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, stdout)
	}
	if err := v.Validar(); err != nil || v.Resultado != veredito.Aprovado || v.Estatisticas.Total != 1 {
		t.Fatalf("veredito %+v: %v", v, err)
	}
	laudo, err := os.ReadFile(report)
	if err != nil || !strings.Contains(string(laudo), "# Laudo Parallax "+v.ID) {
		t.Fatalf("report: %v %q", err, laudo)
	}
	if ids := execucoesGravadas(t, runs); len(ids) != 1 || ids[0] != v.ID {
		t.Fatalf("histórico %v", ids)
	}
}

func TestCheckSemTrocasComparadas(t *testing.T) {
	base := servidorCheck(t, 200, "ok")
	runs := filepath.Join(t.TempDir(), "runs")
	codigo, _, stderr := checarTeste("--capture", capturaCheck(t, `{"metodo":"POST","caminho":"/x","status":200}`),
		"--base", base, "--candidate", base, "--runs-dir", runs)
	if codigo != 4 || !strings.Contains(stderr, "nenhuma troca comparada") {
		t.Fatalf("código %d, stderr %q", codigo, stderr)
	}
	if ids := execucoesGravadas(t, runs); len(ids) != 0 {
		t.Fatalf("gravou histórico sem evidência: %v", ids)
	}
}

// Falha ao gravar o histórico sai com 4, mas o laudo continua aparecendo.
func TestCheckFalhaNoHistorico(t *testing.T) {
	base := servidorCheck(t, 200, "ok")
	runs := filepath.Join(t.TempDir(), "arquivo")
	if err := os.WriteFile(runs, []byte("não é pasta"), 0o600); err != nil {
		t.Fatal(err)
	}
	codigo, stdout, stderr := checarTeste("--capture", capturaCheck(t), "--base", base, "--candidate", base, "--runs-dir", runs)
	if codigo != 4 || !strings.Contains(stderr, "histórico") || !strings.Contains(stdout, "**APROVADO**") {
		t.Fatalf("código %d, stderr %q, stdout %q", codigo, stderr, stdout)
	}
}

func TestCheckFlagsInvalidas(t *testing.T) {
	base := servidorCheck(t, 200, "ok")
	captura := capturaCheck(t)
	casos := [][]string{
		{},
		{"--capture", captura},
		{"--capture", captura, "--base", base, "--candidate", base, "--report", captura},
		{"--capture", captura, "--base", base, "--candidate", base, "--out", "x.json"},
		{"--capture", captura, "--base", base, "--candidate", base, "sobra"},
		{"--capture", filepath.Join(t.TempDir(), "nao-existe.jsonl"), "--base", base, "--candidate", base},
	}
	for _, args := range casos {
		runs := filepath.Join(t.TempDir(), "runs")
		codigo, _, stderr := checarTeste(append(args, "--runs-dir", runs)...)
		if codigo != 4 || strings.TrimSpace(stderr) == "" {
			t.Errorf("%v: código %d, stderr %q", args, codigo, stderr)
		}
		if ids := execucoesGravadas(t, runs); len(ids) != 0 {
			t.Errorf("%v: gravou histórico", args)
		}
	}
	if dados, _ := os.ReadFile(captura); !strings.Contains(string(dados), "/pedidos") {
		t.Fatal("--report sobrescreveu a captura")
	}
}

// Regressão com falha no histórico sai com 4, não 1: o CI não pode perder o
// registro sem ver. A senha da URL não vai para o execucao.json.
func TestCheckRegressaoComFalhaNoHistoricoESenha(t *testing.T) {
	base := servidorCheck(t, 200, "ok")
	quebrada := servidorCheck(t, 500, "erro")
	runs := filepath.Join(t.TempDir(), "arquivo")
	os.WriteFile(runs, []byte("x"), 0o600)
	codigo, stdout, _ := checarTeste("--capture", capturaCheck(t), "--base", base, "--candidate", quebrada, "--runs-dir", runs)
	if codigo != 4 || !strings.Contains(stdout, "**REGRESSAO**") {
		t.Fatalf("código %d, stdout %q", codigo, stdout)
	}

	runs = filepath.Join(t.TempDir(), "runs")
	comSenha := strings.Replace(base, "http://", "http://ci:s3cr3t@", 1)
	if codigo, _, stderr := checarTeste("--capture", capturaCheck(t), "--base", comSenha, "--candidate", base, "--runs-dir", runs); codigo != 0 {
		t.Fatalf("código %d: %s", codigo, stderr)
	}
	ids := execucoesGravadas(t, runs)
	dados, err := os.ReadFile(filepath.Join(runs, ids[0], "execucao.json"))
	if err != nil || strings.Contains(string(dados), "s3cr3t") || !strings.Contains(string(dados), "ci:xxxxx@") {
		t.Fatalf("execucao.json: %v %s", err, dados)
	}
}
