package repeticao

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"

	"github.com/leonardolermen/parallax/internal/captura"
	"github.com/leonardolermen/parallax/internal/platform"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

type resposta struct {
	veredito.Lado
	Corpo      []byte
	Cabecalhos http.Header
	Incompleta bool
}

func urlTroca(base string, t captura.Troca) string {
	b, _ := url.Parse(base)
	b.Path = strings.TrimRight(b.Path, "/") + t.Caminho
	if b.RawPath != "" {
		prefix, _ := url.Parse(base)
		p := &url.URL{Path: t.Caminho}
		b.RawPath = strings.TrimRight(prefix.EscapedPath(), "/") + p.EscapedPath()
	}
	b.RawQuery = t.Query
	b.ForceQuery = false
	return b.String()
}
func headers(t captura.Troca, c Config) http.Header {
	h := http.Header{}
	for k, vs := range t.CabecalhosRequisicao {
		for _, v := range vs {
			if v != captura.Mascara {
				h.Add(k, v)
			}
		}
	}
	removerHop(h)
	for k, vs := range c.Cabecalhos {
		h.Del(k)
		for _, v := range vs {
			if v != captura.Mascara {
				h.Add(k, v)
			}
		}
	}
	removerHop(h)
	return h
}
func removerHop(h http.Header) {
	// Connection can nominate additional hop-by-hop headers.
	for _, v := range h.Values("Connection") {
		for _, k := range strings.Split(v, ",") {
			h.Del(strings.TrimSpace(k))
		}
	}
	for _, k := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade", "Host", "Content-Length", "Traceparent", "Tracestate"} {
		h.Del(k)
	}
}
func novoCliente(c Config) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableCompression = true
	return &http.Client{Timeout: c.Timeout, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func enviar(ctx context.Context, client *http.Client, c Config, base string, t captura.Troca) resposta {
	var out resposta
	inicio := platform.Instante()
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		out.Erro = "falha"
		return out
	}
	out.TraceID = hex.EncodeToString(raw[:16])
	req, err := http.NewRequestWithContext(ctx, t.Metodo, urlTroca(base, t), bytes.NewReader(t.CorpoRequisicao))
	if err != nil {
		out.Erro = "falha"
		return out
	}
	req.Header = headers(t, c)
	req.Header.Set("Traceparent", "00-"+out.TraceID+"-"+hex.EncodeToString(raw[16:])+"-01")
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	resp, err := client.Do(req)
	if err == nil {
		out.Status = resp.StatusCode
		out.Cabecalhos = resp.Header.Clone()
		var r io.Reader = resp.Body
		var gz *gzip.Reader
		semCorpo := t.Metodo == "HEAD" || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified
		if !semCorpo && strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
			gz, err = gzip.NewReader(resp.Body)
			if err == nil {
				r = gz
			}
		}
		if err == nil {
			out.Corpo, err = io.ReadAll(io.LimitReader(r, c.MaxCorpo+1))
			if int64(len(out.Corpo)) > c.MaxCorpo {
				out.Incompleta = true
				out.Corpo = out.Corpo[:c.MaxCorpo]
			}
		}
		if gz != nil {
			_ = gz.Close()
		}
		_ = resp.Body.Close()
	}
	out.LatenciaMs = platform.DecorridoMs(inicio)
	if err != nil {
		out.Incompleta = true
		out.Erro = erroHTTP(err)
	}
	out.Lado.Corpo = resumo(out.Corpo)
	if out.Incompleta {
		out.Lado.Corpo += " [comparacao incompleta]"
	}
	return out
}
func erroHTTP(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelado"
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.Errno(10061)), strings.Contains(strings.ToLower(err.Error()), "refused"):
		return "conexao_recusada"
	default:
		return "falha"
	}
}
func resumo(b []byte) string {
	s := []rune(strings.ToValidUTF8(string(b), "�"))
	if len(s) > 500 {
		s = s[:500]
	}
	return string(s)
}
