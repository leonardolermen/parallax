package julgamento

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestHistoricoArquivosAusentes(t *testing.T) {
	for _, arquivo := range []string{"execucao.json", "veredito.json", "laudo.md"} {
		t.Run(arquivo, func(t *testing.T) {
			testarHistoricoDegradado(t, arquivo, os.Remove, false)
		})
	}
}

func TestHistoricoJSONCorrompido(t *testing.T) {
	for _, arquivo := range []string{"execucao.json", "veredito.json"} {
		t.Run(arquivo, func(t *testing.T) {
			corromper := func(caminho string) error {
				return os.WriteFile(caminho, []byte("{"), 0o600)
			}
			testarHistoricoDegradado(t, arquivo, corromper, true)
		})
	}
}

func testarHistoricoDegradado(t *testing.T, arquivo string, danificar func(string) error, corrompido bool) {
	t.Helper()
	h := Historico{Dir: t.TempDir()}
	const idIntegro = "01JD0000000000000000000001"
	data := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	salvarTeste(t, h, idTeste, data.Add(time.Hour))
	eSalva, vSalvo := salvarTeste(t, h, idIntegro, data)

	if err := danificar(filepath.Join(h.Dir, idTeste, arquivo)); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := h.Ler(idTeste)
	if corrompido {
		if err == nil || errors.Is(err, ErrNaoEncontrada) || !strings.Contains(err.Error(), idTeste) {
			t.Errorf("Ler(%s) = %v, esperado erro de leitura identificando a execução", idTeste, err)
		}
	} else if !errors.Is(err, ErrNaoEncontrada) {
		t.Errorf("Ler(%s) = %v, esperado ErrNaoEncontrada", idTeste, err)
	}

	listadas, avisos := h.Listar()
	var ids []string
	for _, e := range listadas {
		ids = append(ids, e.ID)
	}
	idsEsperados := []string{idTeste, idIntegro}
	if arquivo == "execucao.json" {
		idsEsperados = []string{idIntegro}
	}
	if !slices.Equal(ids, idsEsperados) {
		t.Errorf("ids de Listar = %v, esperado %v", ids, idsEsperados)
	}
	if arquivo == "execucao.json" && corrompido {
		if len(avisos) != 1 || !strings.Contains(avisos[0].Error(), idTeste) {
			t.Errorf("avisos de Listar = %v, esperado um aviso para %s", avisos, idTeste)
		}
	} else if len(avisos) != 0 {
		t.Errorf("avisos inesperados de Listar = %v", avisos)
	}

	e, v, laudo, err := h.Ler(idIntegro)
	if err != nil {
		t.Fatalf("Ler(%s) = %v", idIntegro, err)
	}
	if !reflect.DeepEqual(e, eSalva) || !reflect.DeepEqual(v, vSalvo) || string(laudo) != "# laudo "+idIntegro {
		t.Errorf("execução íntegra alterada: execução=%+v, veredito=%+v, laudo=%q", e, v, laudo)
	}
}
