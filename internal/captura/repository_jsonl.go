package captura

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrEscritorFechado é devolvido por Escritor.Escrever depois de Fechar.
var ErrEscritorFechado = errors.New("motor: escritor fechado")

// Escritor grava trocas em JSONL, uma por linha. É seguro para uso
// concorrente: cada linha é escrita inteira, sem intercalar.
type Escritor struct {
	mu      sync.Mutex
	buf     *bufio.Writer
	fechado bool
}

// NovoEscritor cria um Escritor sobre w. O Escritor não fecha w.
func NovoEscritor(w io.Writer) *Escritor {
	return &Escritor{buf: bufio.NewWriter(w)}
}

// Escrever grava uma troca como uma linha JSON terminada em "\n".
// Corpos nil são gravados como "" (nunca null).
func (e *Escritor) Escrever(t Troca) error {
	if t.CorpoRequisicao == nil {
		t.CorpoRequisicao = []byte{}
	}
	if t.CorpoResposta == nil {
		t.CorpoResposta = []byte{}
	}
	var linha bytes.Buffer
	enc := json.NewEncoder(&linha)
	enc.SetEscapeHTML(false) // query e corpos legíveis: "&" em vez de "&"
	// Encode termina a linha com "\n".
	if err := enc.Encode(t); err != nil {
		return fmt.Errorf("motor: serializar troca: %w", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fechado {
		return ErrEscritorFechado
	}
	if _, err := e.buf.Write(linha.Bytes()); err != nil {
		return fmt.Errorf("motor: escrever troca: %w", err)
	}
	return nil
}

// Flush descarrega no io.Writer o que está no buffer.
func (e *Escritor) Flush() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.buf.Flush()
}

// Fechar faz flush e passa a recusar escritas com ErrEscritorFechado.
// Não fecha o io.Writer de baixo. Chamar de novo não tem efeito.
func (e *Escritor) Fechar() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fechado {
		return nil
	}
	e.fechado = true
	return e.buf.Flush()
}

// Leitor lê trocas de uma captura JSONL, linha a linha, sem limite de tamanho
// de linha. Linhas em branco são ignoradas.
type Leitor struct {
	r     *bufio.Reader
	linha int
}

// NovoLeitor cria um Leitor sobre r.
func NovoLeitor(r io.Reader) *Leitor {
	return &Leitor{r: bufio.NewReader(r)}
}

// Proxima devolve a próxima troca, ou io.EOF quando a captura acabou.
// Uma linha inválida devolve um erro que cita o número da linha.
func (l *Leitor) Proxima() (Troca, error) {
	for {
		dados, err := l.r.ReadBytes('\n')
		if len(dados) == 0 && err != nil {
			if errors.Is(err, io.EOF) {
				return Troca{}, io.EOF
			}
			return Troca{}, fmt.Errorf("motor: ler captura: %w", err)
		}
		l.linha++
		if len(bytes.TrimSpace(dados)) == 0 {
			if err != nil && !errors.Is(err, io.EOF) {
				return Troca{}, fmt.Errorf("motor: ler captura: %w", err)
			}
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return Troca{}, fmt.Errorf("motor: ler captura na linha %d: %w", l.linha, err)
		}
		var t Troca
		if errJSON := json.Unmarshal(dados, &t); errJSON != nil {
			return Troca{}, fmt.Errorf("motor: captura inválida na linha %d: %w", l.linha, errJSON)
		}
		return t, nil
	}
}
