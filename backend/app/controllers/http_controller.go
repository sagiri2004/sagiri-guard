package controllers

import (
	"fmt"
	"io"
	"net/http"
)

type HTTPController struct{}

func NewHTTPController() *HTTPController {
	return &HTTPController{}
}

func (c *HTTPController) Ping(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("pong"))
}

func (c *HTTPController) Echo(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (c *HTTPController) Update(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid form"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("updated"))
}

func (c *HTTPController) DeleteResource(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (c *HTTPController) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid multipart form"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing file"))
		return
	}
	defer file.Close()
	size, _ := io.Copy(io.Discard, file)
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, "received %s (%d bytes)", header.Filename, size)
}
