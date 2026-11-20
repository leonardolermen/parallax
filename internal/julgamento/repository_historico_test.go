package julgamento

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/leonardolermen/parallax/pkg/veredito"
)

func salvarTeste(t *testing.T, h Historico, id string, data time.Time) (Execucao, veredito.Veredito) {
	t.Helper()
	r := resultadoTeste(3, achado(veredito.Diferenca, veredito.Branch, "GET /x"))
	r.ID = id
	v, err := Julgar(r)
	if err != nil {
		t.Fatal(err)
	}
	e := execucaoTeste(v)
	e.Data = data
	e.Estatisticas = r.Estatisticas
	if err := h.Salvar(e, v, []byte("# laudo "+id)); err != nil {
		t.Fatal(err)
	}
	return e, v
}

func TestHistoricoIdaEVolta(t *testing.T) {
	h := Historico{Dir: filepath.Join(t.TempDir(), "runs")}
	e, v := salvarTeste(t, h, idTeste, time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC))
	le, lv, laudo, err := h.Ler(idTeste)
	if err != nil {
		t.Fatal(err)
	}
	if le.ID != e.ID || !le.Data.Equal(e.Data) || le.Captura != e.Captura || le.Resultado != veredito.Revisar || le.Estatisticas.Puladas != 3 {
		t.Fatalf("execucao lida: %+v", le)
	}
	if lv.ID != v.ID || lv.Resultado != v.Resultado || len(lv.Achados) != 1 || lv.Validar() != nil {
		t.Fatalf("veredito lido: %+v", lv)
	}
	if string(laudo) != "# laudo "+idTeste {
		t.Fatalf("laudo lido: %q", laudo)
	}
	sobras, _ := filepath.Glob(filepath.Join(h.Dir, idTeste, "*.tmp"))
	if len(sobras) != 0 {
		t.Fatalf("arquivos temporários sobrando: %v", sobras)
	}
	if runtime.GOOS != "windows" {
		for _, nome := range []string{"veredito.json", "laudo.md", "execucao.json"} {
			info, err := os.Stat(filepath.Join(h.Dir, idTeste, nome))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Errorf("%s: %v %v", nome, info.Mode(), err)
			}
		}
		if info, err := os.Stat(filepath.Join(h.Dir, idTeste)); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("pasta: %v %v", info.Mode(), err)
		}
	}
}

func TestHistoricoListar(t *testing.T) {
	h := Historico{Dir: filepath.Join(t.TempDir(), "runs")}
	if es, errs := h.Listar(); len(es) != 0 || len(errs) != 0 {
		t.Fatalf("pasta inexistente: %v %v", es, errs)
	}
	dia := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	salvarTeste(t, h, "01JD0000000000000000000001", dia)
	salvarTeste(t, h, "01JD0000000000000000000003", dia)
	salvarTeste(t, h, "01JD0000000000000000000002", dia.Add(time.Hour))
	corrompida := filepath.Join(h.Dir, "01JD0000000000000000000009")
	if err := os.MkdirAll(corrompida, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corrompida, "execucao.json"), []byte("{lixo"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Pasta sem execucao.json (gravação interrompida) e nome inválido são ignorados.
	os.MkdirAll(filepath.Join(h.Dir, "01JD0000000000000000000008"), 0o700)
	os.MkdirAll(filepath.Join(h.Dir, "outra-coisa"), 0o700)

	es, errs := h.Listar()
	if len(errs) != 1 {
		t.Fatalf("erros: %v", errs)
	}
	var ids []string
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	want := []string{"01JD0000000000000000000002", "01JD0000000000000000000003", "01JD0000000000000000000001"}
	if len(ids) != 3 || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Fatalf("ordem: %v", ids)
	}
}

func TestHistoricoLerInvalido(t *testing.T) {
	h := Historico{Dir: filepath.Join(t.TempDir(), "runs")}
	salvarTeste(t, h, idTeste, time.Now())
	for _, id := range []string{"../x", "..", "", "01JD000000000000000000000Z", "01jd0000000000000000000000/..", "01JD0000000000000000000099"} {
		if _, _, _, err := h.Ler(id); !errors.Is(err, ErrNaoEncontrada) {
			t.Errorf("Ler(%q): %v", id, err)
		}
	}
	if err := h.Salvar(Execucao{ID: "../fora"}, veredito.Veredito{}, nil); err == nil {
		t.Error("Salvar aceitou id inválido")
	}
}
