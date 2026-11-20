package main

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// Perder o erro de Flush na listagem ou de Write no show faz a CLI sinalizar sucesso.
func TestHistoryErroSaida(t *testing.T) {
	alvo := servidorCheck(t, http.StatusOK, "ok")
	runs := filepath.Join(t.TempDir(), "runs")
	if codigo, _, stderr := checarTeste(argsCheckErros(capturaCheck(t), alvo, runs)...); codigo != 0 {
		t.Fatalf("preparando histórico: código %d, stderr %q", codigo, stderr)
	}
	ids := execucoesGravadas(t, runs)
	if len(ids) != 1 {
		t.Fatalf("esperava uma execução gravada, obtive %v", ids)
	}
	casos := []struct {
		nome string
		args []string
	}{
		{nome: "listagem"},
		{nome: "show Markdown", args: []string{"show", ids[0]}},
		{nome: "show JSON", args: []string{"show", ids[0], "--json"}},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			args := append([]string{"--runs-dir", runs}, caso.args...)
			var stderr bytes.Buffer
			codigo := historico(args, stdoutComErro{}, &stderr)
			if codigo != 4 || !strings.Contains(stderr.String(), errStdoutCLI.Error()) {
				t.Fatalf("código %d, stderr %q", codigo, stderr.String())
			}
		})
	}
}
