package julgamento

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/leonardolermen/parallax/pkg/veredito"
)

// ErrNaoEncontrada: o id não é válido ou não existe no histórico.
var ErrNaoEncontrada = errors.New("execução não encontrada")

// Arquivos de cada execução, em <Dir>/<id>/. execucao.json é gravado por
// último e marca a execução como completa.
const (
	arquivoVeredito = "veredito.json"
	arquivoLaudo    = "laudo.md"
	arquivoExecucao = "execucao.json"
)

// alfabetoULID é o base32 de Crockford usado em platform.NovoID.
const alfabetoULID = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Historico guarda as execuções em arquivos JSON numa pasta local. A pasta de
// cada execução tem permissão 0700 e os arquivos 0600, porque o curl de
// reprodução pode levar cabeçalhos passados com --header.
type Historico struct{ Dir string }

// idValido aceita só ULIDs de 26 caracteres; isso também impede caminhos como
// "../x" de sair da pasta do histórico.
func idValido(id string) bool {
	if len(id) != 26 {
		return false
	}
	for _, c := range id {
		if !strings.ContainsRune(alfabetoULID, c) {
			return false
		}
	}
	return true
}

// Salvar grava veredito.json, laudo.md e execucao.json da execução e.ID. Cada
// arquivo é escrito num temporário na mesma pasta e renomeado.
func (h Historico) Salvar(e Execucao, v veredito.Veredito, laudo []byte) error {
	if !idValido(e.ID) {
		return fmt.Errorf("histórico: id inválido %q", e.ID)
	}
	dir := filepath.Join(h.Dir, e.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("histórico: %w", err)
	}
	vj, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("histórico: %w", err)
	}
	ej, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("histórico: %w", err)
	}
	for _, a := range []struct {
		nome  string
		dados []byte
	}{{arquivoVeredito, append(vj, '\n')}, {arquivoLaudo, laudo}, {arquivoExecucao, append(ej, '\n')}} {
		if err := gravarAtomico(dir, a.nome, a.dados); err != nil {
			return fmt.Errorf("histórico: %w", err)
		}
	}
	return nil
}

func gravarAtomico(dir, nome string, dados []byte) error {
	f, err := os.CreateTemp(dir, nome+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(dados)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		err = os.Chmod(tmp, 0o600)
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, nome))
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// Listar devolve as execuções completas, da mais recente para a mais antiga
// (empate decidido pelo id). Pasta inexistente é histórico vazio. Uma entrada
// ilegível entra na lista de erros e as outras continuam.
func (h Historico) Listar() ([]Execucao, []error) {
	entradas, err := os.ReadDir(h.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{fmt.Errorf("histórico: %w", err)}
	}
	var es []Execucao
	var errs []error
	for _, d := range entradas {
		if !d.IsDir() || !idValido(d.Name()) {
			continue
		}
		var e Execucao
		err := lerJSON(filepath.Join(h.Dir, d.Name(), arquivoExecucao), &e)
		if errors.Is(err, os.ErrNotExist) {
			continue // gravação interrompida antes do execucao.json
		}
		if err == nil && e.ID != d.Name() {
			err = fmt.Errorf("id %q não bate com a pasta", e.ID)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("histórico: %s: %w", d.Name(), err))
			continue
		}
		es = append(es, e)
	}
	sort.Slice(es, func(i, j int) bool {
		if !es[i].Data.Equal(es[j].Data) {
			return es[i].Data.After(es[j].Data)
		}
		return es[i].ID > es[j].ID
	})
	return es, errs
}

// Ler devolve a execução, o veredito e o laudo gravados para id. Id inválido
// ou ausente dá ErrNaoEncontrada.
func (h Historico) Ler(id string) (Execucao, veredito.Veredito, []byte, error) {
	var e Execucao
	var v veredito.Veredito
	if !idValido(id) {
		return e, v, nil, fmt.Errorf("%w: id inválido %q", ErrNaoEncontrada, id)
	}
	dir := filepath.Join(h.Dir, id)
	naoEncontrada := func(err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNaoEncontrada, id)
		}
		return fmt.Errorf("histórico: %s: %w", id, err)
	}
	if err := lerJSON(filepath.Join(dir, arquivoExecucao), &e); err != nil {
		return e, v, nil, naoEncontrada(err)
	}
	if err := lerJSON(filepath.Join(dir, arquivoVeredito), &v); err != nil {
		return e, v, nil, naoEncontrada(err)
	}
	laudo, err := os.ReadFile(filepath.Join(dir, arquivoLaudo))
	if err != nil {
		return e, v, nil, naoEncontrada(err)
	}
	return e, v, laudo, nil
}

func lerJSON(caminho string, destino any) error {
	dados, err := os.ReadFile(caminho)
	if err != nil {
		return err
	}
	return json.Unmarshal(dados, destino)
}
