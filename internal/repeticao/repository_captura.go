package repeticao

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/leonardolermen/parallax/internal/captura"
)

func carregar(ctx context.Context, r io.Reader) ([]captura.Troca, error) {
	if r == nil {
		return nil, fmt.Errorf("motor: captura ausente")
	}
	l := captura.NovoLeitor(r)
	var ts []captura.Troca
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("motor: carregar captura: %w", err)
		}
		t, err := l.Proxima()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if err := validarTroca(t); err != nil {
			return nil, fmt.Errorf("motor: troca %d: %w", len(ts)+1, err)
		}
		ts = append(ts, t)
	}
	if len(ts) == 0 {
		return nil, fmt.Errorf("motor: captura vazia")
	}
	return ts, nil
}
func validarTroca(t captura.Troca) error {
	if !token(t.Metodo) {
		return fmt.Errorf("metodo HTTP invalido")
	}
	if !strings.HasPrefix(t.Caminho, "/") || strings.HasPrefix(t.Caminho, "//") {
		return fmt.Errorf("caminho invalido")
	}
	for _, r := range t.Caminho {
		if r < 32 || r == 127 {
			return fmt.Errorf("caminho invalido")
		}
	}
	if strings.ContainsAny(t.Query, "#\r\n\t ") {
		return fmt.Errorf("query invalida")
	}
	if _, err := url.ParseRequestURI("/?" + t.Query); err != nil {
		return fmt.Errorf("query invalida: %w", err)
	}
	if err := validarCabecalhos(t.CabecalhosRequisicao); err != nil {
		return err
	}
	return validarCabecalhos(t.CabecalhosResposta)
}
func pular(t captura.Troca, c Config) bool {
	return t.CorpoRequisicaoTruncado || (!c.PermitirInseguros && t.Metodo != "GET" && t.Metodo != "HEAD" && t.Metodo != "OPTIONS")
}
