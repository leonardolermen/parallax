package captura

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"sync"
)

// copiaLimitada guarda até limite bytes e marca truncado se vier mais.
// É segura para uso concorrente (o Transport pode ler o corpo da requisição
// em outra goroutine).
type copiaLimitada struct {
	mu       sync.Mutex
	limite   int64
	buf      bytes.Buffer
	truncado bool
}

func (c *copiaLimitada) guardar(b []byte) {
	if len(b) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	espaco := c.limite - int64(c.buf.Len())
	if int64(len(b)) > espaco {
		if espaco > 0 {
			c.buf.Write(b[:espaco])
		}
		c.truncado = true
		return
	}
	c.buf.Write(b)
}

// conteudo devolve uma cópia do que foi guardado (nunca nil) e se truncou.
func (c *copiaLimitada) conteudo() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	saida := make([]byte, c.buf.Len())
	copy(saida, c.buf.Bytes())
	return saida, c.truncado
}

// corpoTee envolve o corpo da requisição: tudo o que o proxy lê segue para o
// upstream e uma cópia, até o limite, fica guardada.
type corpoTee struct {
	io.ReadCloser
	copia *copiaLimitada
}

func (t *corpoTee) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	t.copia.guardar(p[:n])
	return n, err
}

// respostaGravada envolve o ResponseWriter do cliente: tudo segue direto
// para o cliente (inclusive Flush, para SSE) e uma cópia do status, dos
// cabeçalhos e do corpo (até o limite) fica guardada.
type respostaGravada struct {
	w          http.ResponseWriter
	status     int
	cabecalhos http.Header
	corpo      *copiaLimitada
	escreveu   bool // já enviou o status final
	sequestrou bool // conexão sequestrada (ex.: upgrade para WebSocket)
}

func (r *respostaGravada) Header() http.Header { return r.w.Header() }

func (r *respostaGravada) WriteHeader(code int) {
	if r.escreveu {
		r.w.WriteHeader(code) // chamada supérflua: o servidor decide o que fazer
		return
	}
	if code >= 100 && code < 200 {
		// 1xx informativo (ex.: 103 Early Hints) não é o status final.
		r.w.WriteHeader(code)
		return
	}
	r.escreveu = true
	r.status = code
	r.cabecalhos = r.w.Header().Clone()
	r.w.WriteHeader(code)
}

func (r *respostaGravada) Write(b []byte) (int, error) {
	if !r.escreveu {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.w.Write(b)
	r.corpo.guardar(b[:n])
	return n, err
}

// Flush repassa o flush ao cliente (necessário para SSE e streaming).
func (r *respostaGravada) Flush() {
	if !r.escreveu {
		r.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(r.w).Flush()
}

// Hijack repassa o sequestro da conexão (usado pelo proxy em upgrades) e
// marca a troca para não ser gravada.
func (r *respostaGravada) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	r.sequestrou = true
	return http.NewResponseController(r.w).Hijack()
}

// Unwrap permite que http.ResponseController alcance o ResponseWriter
// original (prazos de leitura e escrita, por exemplo).
func (r *respostaGravada) Unwrap() http.ResponseWriter { return r.w }
