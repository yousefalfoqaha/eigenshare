package page

import (
	"net/http"
	"net/url"
	"strings"

	"eigenshare/internal/security"
)

type banner struct {
	Kind    string
	Message string
}

type flash struct {
	cookie *security.CookieManager
}

func newFlash(cookie *security.CookieManager) *flash {
	return &flash{cookie: cookie}
}

func (f *flash) Success(w http.ResponseWriter, msg string) { f.set(w, "success", msg) }
func (f *flash) Info(w http.ResponseWriter, msg string)    { f.set(w, "info", msg) }
func (f *flash) Error(w http.ResponseWriter, msg string)   { f.set(w, "error", msg) }

func (f *flash) set(w http.ResponseWriter, kind, msg string) {
	f.cookie.Set(w, kind+":"+url.QueryEscape(msg))
}

func (f *flash) Pop(w http.ResponseWriter, r *http.Request) *banner {
	value, ok := f.cookie.Get(r)
	if !ok {
		return nil
	}
	f.cookie.Clear(w)

	kind, raw, found := strings.Cut(value, ":")
	if !found {
		return &banner{Kind: "error", Message: value}
	}
	msg, err := url.QueryUnescape(raw)
	if err != nil {
		msg = raw
	}
	return &banner{Kind: kind, Message: msg}
}
