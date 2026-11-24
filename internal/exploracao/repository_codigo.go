package exploracao

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Limites da leitura do código: arquivos maiores que isso (gerados,
// minificados) e pastas de dependências ficam de fora.
const (
	maxArquivoCodigo = 1 << 20
	maxArquivos      = 20000
)

var extensoesCodigo = map[string]bool{
	".go": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".jsx": true, ".tsx": true,
	".py": true, ".java": true, ".kt": true, ".rb": true, ".php": true, ".cs": true, ".rs": true,
	".html": true, ".vue": true, ".svelte": true,
}

var pastasIgnoradas = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "target": true,
	".venv": true, "venv": true, "__pycache__": true, "testdata": true, ".parallax": true, "bin": true, "obj": true,
}

var (
	// reRegistro acha as linhas que registram rota ou chamam uma rota do
	// próprio serviço: HandleFunc/Handle (Go), app.get/router.get (Express),
	// @app.route/@app.get (Flask/FastAPI), @GetMapping/@RequestMapping
	// (Spring), path()/re_path() (Django), fetch()/axios (frontend).
	reRegistro = regexp.MustCompile(`(?i)(handle(func|r)?\s*\(|\.(get|head|route|handle|all|group|mount|use)\s*\(|@(app|router|bp|blueprint)\.|@(get|request)mapping|@(get|path)\b|\b(re_)?path\s*\(|\bfetch\s*\(|axios|\bhref\s*=|getjson|eventsource)`)
	// reEscrita marca a linha como de método que muda dado: o caminho dela
	// não entra (a menos que o próprio texto diga "GET /...").
	reEscrita = regexp.MustCompile(`(?i)(\.(post|put|patch|delete)\s*\(|@(app|router|bp)\.(post|put|patch|delete)|@(post|put|patch|delete)mapping|method\s*[:=]\s*["'](post|put|patch|delete)|methods\s*=\s*\[[^\]]*["'](post|put|patch|delete))`)
	reTexto   = regexp.MustCompile("\"([^\"\\n]{1,300})\"|'([^'\\n]{1,300})'|`([^`\\n]{1,300})`")
	reMetodo  = regexp.MustCompile(`^(GET|HEAD|POST|PUT|PATCH|DELETE|OPTIONS)\s+`)
)

// LerRotasDoCodigo percorre as pastas e devolve as rotas GET escritas no
// código, sem repetir. Arquivos de teste ficam de fora.
func LerRotasDoCodigo(pastas []string) ([]Rota, error) {
	vistos := map[string]bool{}
	var rotas []Rota
	lidos := 0
	for _, raiz := range pastas {
		err := filepath.WalkDir(raiz, func(caminho string, d fs.DirEntry, err error) error {
			if err != nil {
				if caminho == raiz {
					return err
				}
				return nil
			}
			nome := d.Name()
			if d.IsDir() {
				if caminho != raiz && (pastasIgnoradas[nome] || strings.HasPrefix(nome, ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if !extensoesCodigo[strings.ToLower(filepath.Ext(nome))] || arquivoDeTeste(nome) {
				return nil
			}
			if lidos >= maxArquivos {
				return filepath.SkipAll
			}
			lidos++
			for _, r := range rotasDoArquivo(caminho) {
				if m := r.Modelo(); !vistos[m] {
					vistos[m] = true
					rotas = append(rotas, r)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return rotas, nil
}

func arquivoDeTeste(nome string) bool {
	n := strings.ToLower(nome)
	return strings.HasSuffix(n, "_test.go") || strings.Contains(n, ".test.") || strings.Contains(n, ".spec.") ||
		strings.HasPrefix(n, "test_") || strings.HasSuffix(n, "_test.py")
}

func rotasDoArquivo(caminho string) []Rota {
	info, err := os.Stat(caminho)
	if err != nil || info.Size() > maxArquivoCodigo {
		return nil
	}
	f, err := os.Open(caminho)
	if err != nil {
		return nil
	}
	defer f.Close()
	var rotas []Rota
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), maxArquivoCodigo)
	for sc.Scan() {
		rotas = append(rotas, RotasDaLinha(sc.Text())...)
	}
	return rotas
}

// RotasDaLinha devolve as rotas GET citadas numa linha de código.
func RotasDaLinha(linha string) []Rota {
	if !reRegistro.MatchString(linha) {
		return nil
	}
	escrita := reEscrita.MatchString(linha)
	var rotas []Rota
	for _, m := range reTexto.FindAllStringSubmatch(linha, -1) {
		texto := m[1] + m[2] + m[3]
		metodo := ""
		if mm := reMetodo.FindStringSubmatch(texto); mm != nil {
			metodo = mm[1]
			texto = strings.TrimSpace(texto[len(mm[0]):])
		}
		switch {
		case metodo == "GET" || metodo == "HEAD":
		case metodo != "":
			continue
		case escrita:
			continue
		}
		if r, ok := AnalisarModelo(texto); ok {
			r.Origem = OrigemCodigo
			rotas = append(rotas, r)
		}
	}
	return rotas
}
