package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leonardolermen/parallax/pkg/veredito"
)

func TestHistory(t *testing.T) {
	base := servidorCheck(t, 200, "ok")
	diferente := servidorCheck(t, 200, "outro")
	runs := filepath.Join(t.TempDir(), "runs")
	captura := capturaCheck(t)
	if codigo, _, stderr := rodar("check", "--capture", captura, "--base", base, "--candidate", base, "--runs-dir", runs); codigo != 0 {
		t.Fatalf("check aprovado: %d %s", codigo, stderr)
	}
	if codigo, _, stderr := rodar("check", "--capture", captura, "--base", base, "--candidate", diferente, "--runs-dir", runs); codigo != 2 {
		t.Fatalf("check revisar: %d %s", codigo, stderr)
	}

	codigo, stdout, stderr := rodar("history", "--runs-dir", runs)
	linhas := strings.Split(strings.TrimSpace(stdout), "\n")
	if codigo != 0 || len(linhas) != 3 {
		t.Fatalf("history: %d %q %q", codigo, stdout, stderr)
	}
	// Mais recente primeiro: o revisar foi o último.
	if !strings.Contains(linhas[1], "revisar") || !strings.Contains(linhas[2], "aprovado") || !strings.Contains(linhas[1], base+" → "+diferente) {
		t.Fatalf("listagem:\n%s", stdout)
	}
	id := strings.Fields(linhas[1])[0]

	codigo, stdout, _ = rodar("history", "show", id, "--runs-dir", runs)
	if codigo != 0 || !strings.Contains(stdout, "# Laudo Parallax "+id) {
		t.Fatalf("show: %d %q", codigo, stdout)
	}
	codigo, stdout, _ = rodar("history", "show", "--json", "--runs-dir", runs, id)
	var v veredito.Veredito
	if err := json.Unmarshal([]byte(stdout), &v); codigo != 0 || err != nil || v.ID != id || v.Resultado != veredito.Revisar {
		t.Fatalf("show --json: %d %v %q", codigo, err, stdout)
	}
	for _, args := range [][]string{
		{"history", "show", "01JD0000000000000000000099", "--runs-dir", runs},
		{"history", "show", "../x", "--runs-dir", runs},
		{"history", "show", "--runs-dir", runs},
		{"history", "show", id, "outro", "--runs-dir", runs},
		{"history", "sobra", "--runs-dir", runs},
	} {
		if codigo, _, stderr := rodar(args...); codigo != 4 || stderr == "" {
			t.Errorf("%v: código %d, stderr %q", args, codigo, stderr)
		}
	}
}

func TestHistoryVazioECorrompido(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	codigo, stdout, stderr := rodar("history", "--runs-dir", runs)
	if codigo != 0 || stdout != "" || !strings.Contains(stderr, "nenhuma execução") {
		t.Fatalf("vazio: %d %q %q", codigo, stdout, stderr)
	}
	ruim := filepath.Join(runs, "01JD0000000000000000000001")
	os.MkdirAll(ruim, 0o700)
	os.WriteFile(filepath.Join(ruim, "execucao.json"), []byte("{"), 0o600)
	codigo, _, stderr = rodar("history", "--runs-dir", runs)
	if codigo != 0 || !strings.Contains(stderr, "01JD0000000000000000000001") {
		t.Fatalf("corrompido: %d %q", codigo, stderr)
	}
}
