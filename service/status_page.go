package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// StatusPageInfo is the data a status page template is rendered with. The
// template is a Go html/template, fields are referenced as `{{ .ChainName }}`,
// `{{ .HeadBlockNum }}`, etc. The same struct is served as JSON on `GET /` with
// `Accept: application/json` so the page can refresh itself.
type StatusPageInfo struct {
	ChainName     string    `json:"chain_name"`
	Ready         bool      `json:"ready"`
	Live          bool      `json:"live"`
	HeadBlockNum  uint64    `json:"head_block_num,omitempty"`
	HeadBlockID   string    `json:"head_block_id,omitempty"`
	HeadBlockTime time.Time `json:"head_block_time,omitzero"`
}

type StatusPageInfoFunc func(ctx context.Context) *StatusPageInfo

// StatusPage serves an HTML page to browsers doing `GET /`. The template file
// is parsed at creation and parsed again whenever its modification time
// changes, so the page can be edited without a restart.
type StatusPage struct {
	infoFunc     StatusPageInfoFunc
	templatePath string
	logger       *zap.Logger

	mu       sync.Mutex
	tmpl     *template.Template
	tmplTime time.Time
}

// NewStatusPage returns nil when templatePath is empty: no status page is
// served and `GET /` keeps its regular behavior.
func NewStatusPage(templatePath string, infoFunc StatusPageInfoFunc, logger *zap.Logger) (*StatusPage, error) {
	if templatePath == "" {
		return nil, nil
	}

	p := &StatusPage{infoFunc: infoFunc, templatePath: templatePath, logger: logger}
	if _, err := p.template(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *StatusPage) template() (*template.Template, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	stat, err := os.Stat(p.templatePath)
	if err != nil {
		return p.tmpl, fmt.Errorf("stat status page template %q: %w", p.templatePath, err)
	}
	if p.tmpl != nil && stat.ModTime().Equal(p.tmplTime) {
		return p.tmpl, nil
	}

	content, err := os.ReadFile(p.templatePath)
	if err != nil {
		return p.tmpl, fmt.Errorf("read status page template %q: %w", p.templatePath, err)
	}
	tmpl, err := template.New("status").Parse(string(content))
	if err != nil {
		return p.tmpl, fmt.Errorf("parse status page template %q: %w", p.templatePath, err)
	}

	p.tmpl = tmpl
	p.tmplTime = stat.ModTime()
	return tmpl, nil
}

// Handler serves the status page on `GET /` when the client accepts HTML, and
// its data as JSON when the client asks for `application/json`. Every other
// request goes to next. gRPC and Connect calls are always POST on a service
// path, so they never match.
func (p *StatusPage) Handler(next http.Handler) http.Handler {
	if p == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/" {
			next.ServeHTTP(w, r)
			return
		}

		accept := r.Header.Get("Accept")
		switch {
		case strings.Contains(accept, "text/html"):
			p.serveHTML(w, r)

		case strings.Contains(accept, "application/json"):
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(p.infoFunc(r.Context()))

		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (p *StatusPage) serveHTML(w http.ResponseWriter, r *http.Request) {
	tmpl, err := p.template()
	if err != nil {
		// Keep serving the last good template while the file is being edited.
		p.logger.Warn("unable to load status page template", zap.Error(err))
	}
	if tmpl == nil {
		http.Error(w, "status page unavailable", http.StatusInternalServerError)
		return
	}

	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, p.infoFunc(r.Context())); err != nil {
		p.logger.Warn("unable to render status page", zap.Error(err))
		http.Error(w, "status page unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
}
