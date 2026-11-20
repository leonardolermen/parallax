package julgamento

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type escritorComFalhaLaudo struct {
	prefixo         bytes.Buffer
	escritasAceitas int
	err             error
}

func (w *escritorComFalhaLaudo) Write(p []byte) (int, error) {
	if w.escritasAceitas == 0 {
		return 0, w.err
	}
	w.escritasAceitas--
	return w.prefixo.Write(p)
}

func TestLaudoErroEscrita(t *testing.T) {
	v, err := Julgar(resultadoTeste(2))
	if err != nil {
		t.Fatal(err)
	}
	e := execucaoTeste(v)
	// Um caminho longo força mais de uma escrita mesmo com o buffer interno.
	e.Captura = strings.Repeat("captura/", 4096) + "entrada.jsonl"
	sentinela := errors.New("falha no destino do laudo")
	for _, c := range []struct {
		nome            string
		escritasAceitas int
	}{
		{"primeira_escrita", 0},
		{"apos_prefixo", 1},
	} {
		t.Run(c.nome, func(t *testing.T) {
			w := &escritorComFalhaLaudo{escritasAceitas: c.escritasAceitas, err: sentinela}
			if err := EscreverLaudo(w, e, v); !errors.Is(err, sentinela) {
				t.Errorf("EscreverLaudo = %v, esperado erro com causa %v", err, sentinela)
			}
			if c.escritasAceitas > 0 && !strings.HasPrefix(w.prefixo.String(), "# Laudo Parallax "+e.ID+"\n") {
				t.Errorf("primeira escrita não aceitou o cabeçalho do laudo (%d bytes aceitos)", w.prefixo.Len())
			}
		})
	}
}
